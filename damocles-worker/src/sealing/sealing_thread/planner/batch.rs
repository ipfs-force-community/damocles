use std::time::{Duration, Instant};

use anyhow::{anyhow, Context, Result};

use crate::{
    metadb::{rocks::RocksMeta, MetaDocumentDB, PrefixedMetaDB, Saved},
    rpc::sealer::{
        AcquireDealsSpec, AllocateSectorSpec, AllocatedSector, Deals, OnChainState, PreCommitOnChainInfo, SealerClient, Seed, SubmitResult,
        WorkerIdentifier,
    },
    sealing::{
        failure::{Failure, IntoFailure, MapErrToFailure},
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
    PCSubmitted { index: usize },
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
    SubmitPC { index: usize },
    ReSubmitPC { index: usize },
    CheckPC { index: usize },
    AssignSeed { index: usize, seed: Seed, delay_to: Instant },
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
    batch_size: u32,
    job: &'a mut Job,
}

impl BatchSealer<'_> {
    pub fn allocate(&self) -> Result<Event, Failure> {
        let maybe_allocated_res = call_rpc! {
            self.job.rpc()=>allocate_sectors_batch(AllocateSectorSpec {
                allowed_miners: Some(self.job.sealing_ctrl.config().allowed_miners.clone()),
                allowed_proof_types: Some(self.job.sealing_ctrl.config().allowed_proof_types.clone()),
                },
                self.batch_size,
            )
        };

        let allocated = match maybe_allocated_res {
            Ok(a) => a,
            Err(e) => {
                tracing::warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Idle);
            }
        };

        if allocated.is_empty() {
            return Ok(Event::Idle);
        }

        Ok(Event::Allocate(allocated))
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

        let sector = self.job.sectors.get(index).context("sector index out of bounds").crit()?;
        let sector_id = sector.base.as_ref().context("sector base required").crit()?.allocated.id.clone();

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

    fn submit_pre_commit(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sectors.get(index).context("sector index out of bounds").crit()?;

        let (sector_id, comm_r, comm_d, ticket) =
            if let (Some(base), Some(pc2out), Some(ticket)) = (&sector.base, &sector.phases.pc2out, sector.phases.ticket.clone()) {
                (base.allocated.clone(), pc2out.comm_r, pc2out.comm_d, ticket)
            } else {
                return Err(anyhow!("PC2 not completed").crit());
            };

        let deals = sector.deals.as_ref().map(|x| x.iter().map(|x| x.id).collect()).unwrap_or_default();

        let pinfo = PreCommitOnChainInfo {
            comm_r,
            comm_d,
            ticket,
            deals,
        };

        let res = call_rpc! {
            self.job.rpc() => submit_pre_commit(sector_id, pinfo, sector.phases.pc2_re_submit,)
        }?;

        // TODO: handle submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitPC { index }),

            SubmitResult::MismatchedSubmission => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort()),

            SubmitResult::FilesMissed => Err(anyhow!("FilesMissed should not happen for pc2 submission: {:?}", res.desc).perm()),
        }
    }

    fn check_pre_commit_state(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sectors.get(index).context("sector index out of bounds").crit()?;
        let sector_id = sector.base.as_ref().map(|b| &b.allocated.id).context("context").crit()?;

        loop {
            let state = call_rpc! {
                self.job.rpc()=>poll_pre_commit_state(sector_id.clone(), )
            }?;

            match state.state {
                OnChainState::Landed => break,
                OnChainState::NotFound => return Err(anyhow!("pre commit on-chain info not found").perm()),

                OnChainState::Failed => {
                    tracing::warn!("pre commit on-chain info failed: {:?}", state.desc);
                    // TODO: make it configurable
                    self.job.sealing_ctrl.wait_or_interrupted(Duration::from_secs(30))?;
                    return Ok(Event::ReSubmitPC { index });
                }

                OnChainState::PermFailed => return Err(anyhow!("pre commit on-chain info permanent failed: {:?}", state.desc).perm()),

                OnChainState::ShouldAbort => return Err(anyhow!("pre commit info will not get on-chain: {:?}", state.desc).abort()),

                OnChainState::Pending | OnChainState::Packed => {}
            }

            tracing::debug!(
                state = ?state.state,
                interval = ?self.job.sealing_ctrl.config().rpc_polling_interval,
                "waiting for next round of polling pre commit state",
            );

            self.job
                .sealing_ctrl
                .wait_or_interrupted(self.job.sealing_ctrl.config().rpc_polling_interval)?;
        }

        tracing::debug!(index = index, "pre commit landed");

        Ok(Event::CheckPC { index })
    }

    fn persist_sector_files(&self, index: usize) -> Result<Event, Failure> {
        todo!()
    }

    fn wait_seed(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sectors.get(index).context("sector index out of bounds").crit()?;
        let sector_id = sector.base.context("sector base required").crit()?.allocated.id;
        let (seed, ) = loop {
            let wait = call_rpc! {
                self.job.rpc()=>wait_seed(sector_id, )
            }?;

            if let Some(seed) = wait.seed {
                break seed;
            };

            if !wait.should_wait || wait.delay == 0 {
                return Err(anyhow!("invalid empty wait_seed response").temp());
            }

            let delay = Duration::from_secs(wait.delay);

            // tracing::debug!(?delay, "waiting for next round of polling seed");
            // self.job.sealing_ctrl.wait_or_interrupted(delay)?;
        };

        Ok(Event::AssignSeed {
            index,
            seed,
            delay_to: Instant::now().checked_add(delay.),
        })
    }
}
