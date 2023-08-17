use anyhow::{Context, Result};

use crate::{
    metadb::{rocks::RocksMeta, MetaDocumentDB, PrefixedMetaDB, Saved},
    rpc::sealer::{AcquireDealsSpec, AllocateSectorSpec, AllocatedSector, Deals, SealerClient, WorkerIdentifier},
    sealing::{
        failure::{Failure, MapErrToFailure},
        sealing_thread::{util::call_rpc, SealingCtrl},
    },
    store::Store,
};

use super::{
    common::sector::{Sector, Trace},
    JobTrait, PlannerTrait,
};

pub(crate) struct Job {
    pub sectors: Saved<Vec<Sector>, &'static str, PrefixedMetaDB<&'static RocksMeta>>,
    _trace: Vec<Trace>,

    pub sealing_ctrl: SealingCtrl<'static>,
    store: &'static Store,
    ident: WorkerIdentifier,

    _trace_meta: MetaDocumentDB<PrefixedMetaDB<&'static RocksMeta>>,
}

impl Job {
    pub fn rpc(&self) -> &SealerClient {
        self.sealing_ctrl.ctx.global.rpc.as_ref()
    }
}

impl JobTrait for Job {
    fn planner(&self) -> &str {
        // Batch planner does not support switching palnner
        "batch"
    }
}

pub enum State {
    Empty,
    Allocated,
    DealsAcquired { index: usize },
    PieceAdded,
    TreeDBuilt,
    TicketAssigned,
    PC1Done,
    PC2Done,
    PCSubmitted,
    PCLanded,
    Persisted,
    PersistanceSubmitted,
    SeedAssigned,
    C1Done,
    C2Done,
    ProofSubmitted,
    Finished,
    Aborted,
}

pub enum Event {
    SetState(State),
    // No specified tasks available from sector_manager.
    Idle,
    Allocate(Vec<AllocatedSector>),
    AcquireDeals { index: usize, deals: Option<Deals> },
}

#[derive(Default)]
pub(crate) struct BatchPlanner;

impl PlannerTrait for BatchPlanner {
    type Job = Job;
    type State = State;
    type Event = Event;

    fn name(&self) -> &str {
        "batch"
    }

    fn plan(&self, evt: &Self::Event, st: &Self::State) -> Result<Self::State> {
        todo!()
    }

    fn exec(&self, job: &mut Self::Job) -> Result<Option<Self::Event>, Failure> {
        todo!()
    }

    fn apply(&self, event: Self::Event, state: Self::State, job: &mut Self::Job) -> Result<()> {
        todo!()
    }
}

struct BatchSealer<'a> {
    job: &'a mut Job,
}

impl BatchSealer<'_> {
    pub fn allocate(&self) -> Result<Event, Failure> {
        let maybe_allocated_res = call_rpc! {
            self.job.rpc()=>allocate_sector(AllocateSectorSpec {
                allowed_miners: Some(self.job.sealing_ctrl.config().allowed_miners.clone()),
                allowed_proof_types: Some(self.job.sealing_ctrl.config().allowed_proof_types.clone()),
            },)
        };

        let maybe_allocated = match maybe_allocated_res {
            Ok(a) => a,
            Err(e) => {
                tracing::warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Idle);
            }
        };

        let sector = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
        };

        Ok(Event::Allocate(sector))
    }

    pub fn acquire_deals(&self, index: usize) -> Result<Event, Failure> {
        let disable_cc = self.job.sealing_ctrl.config().disable_cc;

        if !self.job.sealing_ctrl.config().enable_deals {
            return Ok(if disable_cc {
                Event::Idle
            } else {
                Event::AcquireDeals {
                    index: self.job.sectors.len(),
                    deals: None,
                }
            });
        }
        let spec = AcquireDealsSpec {
            max_deals: self.job.sealing_ctrl.config().max_deals,
            min_used_space: self.job.sealing_ctrl.config().min_deal_space.map(|b| b.get_bytes() as usize),
        };

        let sector = self.job.sectors.get_mut(index).context("sector index out of bounds").crit()?;
        let sector_id = sector.base.context("sector base required").crit()?.allocated.id;

        let deals = call_rpc! {
            self.job.rpc()=>acquire_deals(
                sector_id,
                spec,
            )
        }?;

        let deals_count = deals.as_ref().map(|d| d.len()).unwrap_or(0);

        tracing::debug!(count = deals_count, "pieces acquired");
        Ok(if disable_cc || deals_count > 0 {
            Event::AcquireDeals { index, deals }
        } else {
            Event::Idle
        })
    }

    fn pc1(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn pc2(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn wait_seed(&self) -> Result<Event, Failure> {
        todo!()
    }
}
