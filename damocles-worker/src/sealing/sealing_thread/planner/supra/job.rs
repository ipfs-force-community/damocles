use anyhow::{Context, Result};

use crate::{
    metadb::{rocks::RocksMeta, MetaDocumentDB, PrefixedMetaDB, Saved},
    rpc::sealer::{
        ReportStateReq, SealerClient, SectorFailure, SectorState,
        SectorStateChange, WorkerIdentifier,
    },
    sealing::{
        failure::{Failure, IntoFailure, MapErrToFailure},
        sealing_thread::{
            planner::{common::sector::Trace, JobTrait},
            util::call_rpc,
            SealingCtrl,
        },
    },
    store::Store,
};

use super::{
    event::Event,
    sectors::{Sector, Sectors},
    state::State,
};

const SECTOR_INFO_KEY: &str = "supra_info";
const SECTOR_META_PREFIX: &str = "supra_meta";
const SECTOR_TRACE_PREFIX: &str = "supra_trace";

pub(crate) struct Job {
    pub sectors:
        Saved<Sectors, &'static str, PrefixedMetaDB<&'static RocksMeta>>,
    _trace: Vec<Trace>,

    pub sealing_ctrl: SealingCtrl<'static>,
    pub store: &'static Store,
    ident: WorkerIdentifier,

    num_retry: u32,

    _trace_meta: MetaDocumentDB<PrefixedMetaDB<&'static RocksMeta>>,
}

impl Job {
    const KEY_BLOCK_OFFSET: &str = "supra_block_offset";
    const KEY_NUM_SECTORS: &str = "supra_num_sectors";

    pub fn new(
        sealing_ctrl: SealingCtrl<'static>,
        s: &'static Store,
    ) -> Result<Self> {
        let block_offset = sealing_ctrl
            .sealing_config
            .meta
            .get(Self::KEY_BLOCK_OFFSET)
            .and_then(|n| n.parse().ok())
            .unwrap_or(0u64);

        let num_sectors = sealing_ctrl
            .sealing_config
            .meta
            .get(Self::KEY_NUM_SECTORS)
            .and_then(|n| n.parse().ok())
            .unwrap_or(64usize);

        let sector_meta = PrefixedMetaDB::wrap(SECTOR_META_PREFIX, &s.meta);

        let mut sectors: Saved<Sectors, _, _> =
            Saved::load(SECTOR_INFO_KEY, sector_meta, || {
                Sectors::new(block_offset, num_sectors)
            })
            .context("load sector")?;
        sectors.sync().context("init sync sectors")?;

        let trace_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(
            SECTOR_TRACE_PREFIX,
            &s.meta,
        ));
        let instance = sealing_ctrl.ctx().instance.clone();

        Ok(Self {
            sectors,
            _trace: Vec::with_capacity(16),
            sealing_ctrl,
            store: s,
            ident: WorkerIdentifier {
                instance,
                location: s.location.to_pathbuf(),
            },
            num_retry: 0,
            _trace_meta: trace_meta,
        })
    }

    pub fn rpc(&self) -> &SealerClient {
        self.sealing_ctrl.ctx.global.rpc.as_ref()
    }

    pub fn sector(&self, slot: usize) -> Result<&Sector> {
        self.sectors
            .sectors
            .get(slot)
            .with_context(|| format!("sector slot out of bounds: {}", slot))
    }

    pub fn sector_mut(&mut self, slot: usize) -> Result<&mut Sector> {
        self.sectors
            .sectors
            .get_mut(slot)
            .with_context(|| format!("sector slot out of bounds: {}", slot))
    }

    pub fn report_state(
        &self,
        prev: State,
        next: State,
        event: String,
        fail: Option<SectorFailure>,
    ) -> Result<(), Failure> {
        for sector in prev.range(&self.sectors.sectors) {
            let req = ReportStateReq {
                worker: self.ident.clone(),
                state_change: SectorStateChange {
                    prev: prev.pure(),
                    next: next.pure(),
                    event: event.clone(),
                },
                failure: fail.clone(),
            };

            let _ = call_rpc! {
                self.sealing_ctrl.ctx().global.rpc=>report_state(sector.sector_id.clone(), req,)
            }?;
        }
        Ok(())
    }

    pub fn report_finalized(&self, sectors: &[Sector]) -> Result<(), Failure> {
        for sector in sectors {
            call_rpc! {
                self.sealing_ctrl.ctx.global.rpc => report_finalized(sector.sector_id.clone(),)
            }?;
        }
        Ok(())
    }

    pub fn report_aborted(
        &self,
        sectors: &[Sector],
        reason: String,
    ) -> Result<(), Failure> {
        for sector in sectors {
            call_rpc! {
                self.sealing_ctrl.ctx.global.rpc=>report_aborted(sector.sector_id.clone(), reason.clone(),)
            }?;
        }
        Ok(())
    }

    pub fn finalize(&mut self) -> Result<(), Failure> {
        let block_offset = self
            .sealing_ctrl
            .sealing_config
            .meta
            .get(Self::KEY_BLOCK_OFFSET)
            .and_then(|n| n.parse().ok())
            .unwrap_or(0u64);
        let num_sectors = self
            .sealing_ctrl
            .sealing_config
            .meta
            .get(Self::KEY_NUM_SECTORS)
            .and_then(|n| n.parse().ok())
            .unwrap_or(0usize);
        self.store.cleanup().context("cleanup store").crit()?;
        self.sectors
            .delete(|| Sectors::new(block_offset, num_sectors))
            .context("remove sector")
            .crit()
    }

    pub fn retry(&mut self, temp_err: anyhow::Error) -> Result<(), Failure> {
        if self.num_retry >= self.sealing_ctrl.config().max_retries {
            // reset retry times
            self.num_retry = 0;

            return Err(temp_err.perm());
        }

        tracing::warn!(
            num_retry = self.num_retry,
            "temp error occurred: {:?}",
            temp_err
        );

        self.num_retry += 1;

        tracing::info!(
            interval = ?self.sealing_ctrl.config().recover_interval,
            "wait before recovering"
        );

        self.sealing_ctrl
            .wait_or_interrupted(self.sealing_ctrl.config().recover_interval)?;
        Ok(())
    }
}

impl JobTrait for Job {
    fn planner(&self) -> &str {
        // Batch planner does not support switching palnner
        "batch"
    }
}
