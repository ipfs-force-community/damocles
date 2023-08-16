use std::path::PathBuf;

use anyhow::{Context, Result};
use forest_cid::json::CidJson;

use crate::sealing::failure::{Failure, IntoFailure, MapErrToFailure};
use crate::sealing::paths;
use crate::store::Store;
use crate::types::SealProof;
use crate::{
    metadb::{rocks::RocksMeta, MaybeDirty, MetaDocumentDB, PrefixedMetaDB, Saved},
    rpc::sealer::{ReportStateReq, SectorFailure, SectorID, SectorStateChange, WorkerIdentifier},
};
use crate::{
    rpc::sealer::SealerClient,
    sealing::sealing_thread::{
        entry::Entry,
        planner::JobTrait,
        util::{call_rpc, field_required},
        SealingCtrl,
    },
};

use super::sector::{Sector, Trace};

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

pub(crate) struct Task {
    pub sector: Saved<Sector, &'static str, PrefixedMetaDB<&'static RocksMeta>>,
    _trace: Vec<Trace>,

    pub sealing_ctrl: SealingCtrl<'static>,
    store: &'static Store,
    ident: WorkerIdentifier,

    _trace_meta: MetaDocumentDB<PrefixedMetaDB<&'static RocksMeta>>,
}

// properties
impl Task {
    pub fn sector_id(&self) -> Result<&SectorID, Failure> {
        field_required! {
            sector_id,
            self.sector.base.as_ref().map(|b| &b.allocated.id)
        }

        Ok(sector_id)
    }

    pub fn sector_proof_type(&self) -> Result<&SealProof, Failure> {
        field_required! {
            proof_type,
            self.sector.base.as_ref().map(|b| &b.allocated.proof_type)
        }

        Ok(proof_type)
    }

    pub fn rpc(&self) -> &SealerClient {
        self.sealing_ctrl.ctx.global.rpc.as_ref()
    }
}

impl JobTrait for Task {
    fn planner(&self) -> &str {
        self.sector.plan()
    }
}

impl Task {
    pub fn build(sealing_ctrl: SealingCtrl<'static>, s: &'static Store) -> Result<Self> {
        let sector_meta = PrefixedMetaDB::wrap(SECTOR_META_PREFIX, &s.meta);

        let mut sector: Saved<Sector, _, _> = Saved::load(SECTOR_INFO_KEY, sector_meta, || {
            Sector::new(sealing_ctrl.config().plan().to_string())
        })
        .context("load sector")?;
        sector.sync().context("init sync sector")?;

        let trace_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_TRACE_PREFIX, &s.meta));
        let instance = sealing_ctrl.ctx().instance.clone();

        Ok(Task {
            sector,
            _trace: Vec::with_capacity(16),
            sealing_ctrl,
            store: s,
            ident: WorkerIdentifier {
                instance,
                location: s.location.to_pathbuf(),
            },

            _trace_meta: trace_meta,
        })
    }

    pub fn report_state(&self, state_change: SectorStateChange, fail: Option<SectorFailure>) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.sealing_ctrl.ctx().global.rpc=>report_state(
            sector_id,
            ReportStateReq {
                worker: self.ident.clone(),
                state_change,
                failure: fail,
            },
        )}?;

        Ok(())
    }

    pub fn report_finalized(&self) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.sealing_ctrl.ctx.global.rpc => report_finalized(sector_id,)
        }?;

        Ok(())
    }

    pub fn report_aborted(&self, reason: String) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.sealing_ctrl.ctx.global.rpc=>report_aborted(sector_id, reason,)
        }?;

        Ok(())
    }

    pub fn retry(&mut self, temp_err: anyhow::Error) -> Result<(), Failure> {
        if self.sector.retry >= self.sealing_ctrl.config().max_retries {
            // reset retry times
            self.sync(|s| {
                s.retry = 0;
                Ok(())
            })?;

            return Err(temp_err.perm());
        }

        self.sync(|s| {
            tracing::warn!(retry = s.retry, "temp error occurred: {:?}", temp_err);

            s.retry += 1;

            Ok(())
        })?;

        tracing::info!(
            interval = ?self.sealing_ctrl.config().recover_interval,
            "wait before recovering"
        );

        self.sealing_ctrl.wait_or_interrupted(self.sealing_ctrl.config().recover_interval)?;
        Ok(())
    }

    fn sync<F: FnOnce(&mut MaybeDirty<Sector>) -> Result<()>>(&mut self, modify_fn: F) -> Result<(), Failure> {
        modify_fn(self.sector.inner_mut()).crit()?;
        self.sector.sync().context("sync sector").crit()
    }

    pub fn finalize(&mut self) -> Result<(), Failure> {
        self.store.cleanup().context("cleanup store").crit()?;
        self.sector
            .delete(|| Sector::new(self.sealing_ctrl.config().plan().to_string()))
            .context("remove sector")
            .crit()
    }

    pub fn sector_path(&self, sector_id: &SectorID) -> String {
        paths::sector_path(sector_id)
    }

    pub fn prepared_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(&self.store.data_path, PathBuf::from("prepared").join(self.sector_path(sector_id)))
    }

    pub fn cache_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(&self.store.data_path, paths::cache_dir(sector_id))
    }

    pub fn pc2_running_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, paths::pc2_running_file(sector_id))
    }

    pub fn sealed_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, paths::sealed_file(sector_id))
    }

    pub fn staged_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("unsealed").join(self.sector_path(sector_id)))
    }

    pub fn piece_file(&self, piece_cid: &CidJson) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("unsealed").join(format!("{}", piece_cid.0)))
    }

    pub fn update_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, paths::update_file(sector_id))
    }

    pub fn update_cache_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(&self.store.data_path, paths::update_cache_dir(sector_id))
    }
}
