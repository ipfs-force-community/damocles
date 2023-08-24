use serde::{Deserialize, Serialize};
use std::{
    ops::Add,
    time::{Duration, Instant},
};

use anyhow::{anyhow, Context, Result};

use crate::{
    metadb::{rocks::RocksMeta, MetaDocumentDB, PrefixedMetaDB, Saved},
    rpc::sealer::{
        AcquireDealsSpec, AllocateSectorSpec, AllocatedSector, Deals, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo, SealerClient,
        Seed, SubmitResult, Ticket, WorkerIdentifier,
    },
    sealing::{
        failure::{Failure, IntoFailure, MapErrToFailure},
        sealing_thread::{util::call_rpc, SealingCtrl},
    },
    store::Store,
};

use self::sectors::{Sector, Sectors};

use super::{common::sector::Trace, JobTrait, PlannerTrait};

mod sectors;

pub(crate) struct Job {
    pub sectors: Saved<Sectors, &'static str, PrefixedMetaDB<&'static RocksMeta>>,
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

    pub fn sector(&self, index: usize) -> Result<&Sector> {
        self.sectors
            .sectors
            .get(index)
            .with_context(|| format!("sector index out of bounds: {}", index))
    }
}

impl JobTrait for Job {
    fn planner(&self) -> &str {
        // Batch planner does not support switching palnner
        "batch"
    }
}

#[derive(Serialize, Deserialize)]
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
    PCLanded { index: usize },
    Persisted,
    PersistanceSubmitted,
    SeedAssigned,
    C1Done,
    C2Done,
    ProofSubmitted,
    Finished,
    Aborted,
}

enum MaybeSeed {
    Got(Seed),
    DelayTo(Instant),
}

pub enum Event {
    SetState(State),
    // No specified tasks available from sector_manager.
    Idle,
    Allocate(Vec<AllocatedSector>),
    AcquireDeals { index: usize, deals: Option<Deals> },
    AssignTicket(Ticket),
    SubmitPC { index: usize },
    ReSubmitPC { index: usize },
    CheckPC { index: usize },
    AssignSeed { index: usize, maybe_seed: MaybeSeed },
    SubmitProof { index: usize },
    ReSubmitProof { index: usize },
    Finish,
}

#[derive(Default)]
pub(crate) struct BatchPlanner {
    batch_size: usize,
}

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
        let state = job.sectors.state;
        let inner = BatchSealer { job };

        let batch_size = job.sectors.batch_size;
        match state {
            State::Empty => inner.allocate(),
            State::Allocated => inner.acquire_deals(0),
            State::DealsAcquired { index } if index < batch_size => inner.acquire_deals(index + 1),
            State::DealsAcquired { .. } => inner.add_pieces(),
            State::PieceAdded => inner.build_tree_d(),
            State::TreeDBuilt => inner.assign_ticket(),
            State::TicketAssigned => inner.pc1(),
            State::PC1Done => inner.pc2(),
            State::PC2Done => inner.submit_pre_commit(0),
            State::PCSubmitted { index } if index < batch_size => inner.submit_pre_commit(index + 1),
            State::PCSubmitted { .. } => inner.check_pre_commit_state(0),
            State::PCLanded { index } => {
                
            }
            State::Persisted => todo!(),
            State::PersistanceSubmitted => todo!(),
            State::SeedAssigned => todo!(),
            State::C1Done => todo!(),
            State::C2Done => todo!(),
            State::ProofSubmitted => todo!(),
            State::Finished => todo!(),
            State::Aborted => todo!(),
        }
        .map(Some)
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
            self.job.rpc()=>allocate_sectors_batch(AllocateSectorSpec {
                allowed_miners: Some(self.job.sealing_ctrl.config().allowed_miners.clone()),
                allowed_proof_types: Some(self.job.sealing_ctrl.config().allowed_proof_types.clone()),
                },
                self.job.sectors.batch_size as u32,
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

    fn add_pieces(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn build_tree_d(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn assign_ticket(&self) -> Result<Event, Failure> {
        let sector = self.job.sector(0).crit()?;
        let sector_id = sector.base.context("sector base required").crit()?.allocated.id.clone();

        let ticket = match &sector.phases.ticket {
            // Use the existing ticket when rebuilding sectors
            Some(ticket) => ticket.clone(),
            None => {
                let ticket = call_rpc! {
                    self.job.rpc() => assign_ticket(sector_id,)
                }?;
                tracing::debug!(ticket = ?ticket.ticket.0, epoch = ticket.epoch, "ticket assigned from sector-manager");
                ticket
            }
        };

        Ok(Event::AssignTicket(ticket))
    }

    fn pc1(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn pc2(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn submit_pre_commit(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sector(index).crit()?;

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
        let sector = self.job.sector(index).crit()?;
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
        let sector = self.job.sector(index).crit()?;
        let sector_id = sector.base.as_ref().context("sector base required").crit()?.allocated.id.clone();

        let wait = call_rpc! {
            self.job.rpc()=>wait_seed(sector_id, )
        }?;

        let maybe_seed = match wait.seed {
            Some(seed) => MaybeSeed::Got(seed),
            None => {
                if !wait.should_wait || wait.delay == 0 {
                    return Err(anyhow!("invalid empty wait_seed response").temp());
                }
                MaybeSeed::DelayTo(Instant::now().add(Duration::from_micros(wait.delay)))
            }
        };

        Ok(Event::AssignSeed { index, maybe_seed })
    }

    fn commit1(&self) -> Result<Event, Failure> {
        // cloned_required! {
        //     seed,
        //     self.task.sector.phases.seed
        // }

        // common::commit1_with_seed(self.task, seed).map(Event::C1)
        todo!()
    }

    fn commit2(&self) -> Result<Event, Failure> {
        todo!()
    }

    fn submit_proof(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sectors.get(index).context("sector index out of bounds").crit()?;
        let sector_id = sector.base.as_ref().context("sector base required").crit()?.allocated.id.clone();

        let proof = sector.phases.c2out.clone().context("c2out required").crit()?;

        let info = ProofOnChainInfo { proof: proof.proof.into() };

        let res = call_rpc! {
            self.job.rpc()=>submit_proof(sector_id, info, sector.phases.c2_re_submit,)
        }?;

        // TODO: submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitProof { index }),

            SubmitResult::MismatchedSubmission => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort()),

            SubmitResult::FilesMissed => Err(anyhow!("FilesMissed is not handled currently: {:?}", res.desc).perm()),
        }
    }

    fn check_proof_state(&self, index: usize) -> Result<Event, Failure> {
        let sector = self.job.sector(index).crit()?;
        let sector_id = sector.base.as_ref().context("sector base required").crit()?.allocated.id.clone();

        if !self.job.sealing_ctrl.config().ignore_proof_check {
            loop {
                let state = call_rpc! {
                    self.job.rpc() => poll_proof_state(sector_id.clone(),)
                }?;

                match state.state {
                    OnChainState::Landed => break,
                    OnChainState::NotFound => return Err(anyhow!("proof on-chain info not found").perm()),

                    OnChainState::Failed => {
                        tracing::warn!("proof on-chain info failed: {:?}", state.desc);
                        // TODO: make it configurable
                        self.job.sealing_ctrl.wait_or_interrupted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitProof { index });
                    }

                    OnChainState::PermFailed => return Err(anyhow!("proof on-chain info permanent failed: {:?}", state.desc).perm()),

                    OnChainState::ShouldAbort => return Err(anyhow!("sector will not get on-chain: {:?}", state.desc).abort()),

                    OnChainState::Pending | OnChainState::Packed => {}
                }

                tracing::debug!(
                    state = ?state.state,
                    interval = ?self.job.sealing_ctrl.config().rpc_polling_interval,
                    "waiting for next round of polling proof state",
                );

                self.job
                    .sealing_ctrl
                    .wait_or_interrupted(self.job.sealing_ctrl.config().rpc_polling_interval)?;
            }
        }

        // let cache_dir = self.job.cache_dir(sector_id);
        // let sector_size = allocated.proof_type.sector_size();

        // we should be careful here, use failure as temporary
        // clear_cache(sector_size, cache_dir.as_ref()).temp()?;
        // debug!(
        //     dir = ?&cache_dir,
        //     "clean up unnecessary cached files"
        // );

        Ok(Event::Finish)
    }
}
