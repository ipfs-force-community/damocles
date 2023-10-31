use once_cell::sync::Lazy;
use rayon::prelude::{IntoParallelIterator, ParallelIterator};
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::{
    path::{Path, PathBuf},
    pin::Pin,
    str::FromStr,
    time::Duration,
};
use vc_processors::fil_proofs::{
    cached_filenames_for_sector, clear_cache, compute_comm_d, to_prover_id,
    Commitment, PieceInfo, RegisteredSealProof, SealCommitPhase1Output,
    SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, SectorId,
};

use anyhow::{anyhow, Context, Error, Result};

use crate::{
    metadb::{
        rocks::RocksMeta, MaybeDirty, MetaDocumentDB, PrefixedMetaDB, Saved,
    },
    rpc::sealer::{
        AcquireDealsSpec, AllocateSectorSpec, AllocatedSector, Deals,
        OnChainState, PollPreCommitStateResp, PollProofStateResp,
        PreCommitOnChainInfo, ProofOnChainInfo, SealerClient, SectorFailure,
        SectorID, SectorStateChange, Seed, SubmitResult, Ticket,
        WorkerIdentifier,
    },
    sealing::{
        failure::{
            Failure, FailureContext, IntoFailure, Level, MapErrToFailure,
            TaskAborted,
        },
        paths::{self, sealed_file},
        processor::{C2Input, SupraC1Input, SupraPC1Input, SupraPC2Input},
        sealing_thread::{
            entry::Entry,
            extend_lifetime,
            planner::common,
            util::{call_rpc, cloned_required, field_required},
            Sealer, SealingCtrl, SealingThread, R,
        },
    },
    store::Store,
    watchdog::Ctx,
    SealProof,
};

use self::{
    event::Event,
    job::Job,
    sectors::{Sector, Sectors},
    state::State,
};

use super::{common::sector::Trace, JobTrait, PlannerTrait};

mod event;
mod job;
mod sectors;
mod state;

pub struct SupraSealer {
    job: Job,
    planner: SupraPlanner,
    _store: Pin<Box<Store>>,
}

impl SupraSealer {
    pub(crate) fn new(ctx: &Ctx, st: &SealingThread) -> Result<Self> {
        let location = st.location.as_ref().expect("location must be set");
        let store_path = location.to_pathbuf();
        let store = Box::pin(Store::open(store_path).with_context(|| {
            format!("open store {}", location.as_ref().display())
        })?);

        Ok(Self {
            job: Job::new(st.sealing_ctrl(ctx), unsafe {
                extend_lifetime(&*store.as_ref())
            })?,
            planner: SupraPlanner { num_sectors: 0 },
            _store: store,
        })
    }
}

impl Sealer for SupraSealer {
    fn seal(&mut self, state: Option<&str>) -> Result<R, Failure> {
        let mut event = state
            .and_then(|s| State::from_str(s).ok())
            .map(Event::SetState);
        if let (true, Some(s)) = (event.is_none(), state) {
            tracing::error!("unknown state: {}", s);
        }

        let mut task_idle_count = 0;
        loop {
            let span = tracing::error_span!("supra-seal", ?event);

            let _enter = span.enter();

            let prev = self.job.sectors.state.clone();
            let is_empty = match self.job.sectors.job_id() {
                None => true,
                Some(job_id) => {
                    self.job
                        .sealing_ctrl
                        .ctrl_ctx()
                        .update_state(|cst| {
                            cst.job.id.replace(job_id);
                        })
                        .crit()?;
                    false
                }
            };

            self.job.sealing_ctrl.interrupted()?;

            let handle_res = self.handle(event.take());
            if is_empty {
                if let Some(job_id) = self.job.sectors.job_id() {
                    self.job
                        .sealing_ctrl
                        .ctrl_ctx()
                        .update_state(|cst| {
                            cst.job.id.replace(job_id);
                        })
                        .crit()?;
                }
            } else if self.job.sectors.job_id().is_none() {
                self.job
                    .sealing_ctrl
                    .ctrl_ctx()
                    .update_state(|cst| {
                        cst.job.id.take();
                    })
                    .crit()?;
            }

            match handle_res {
                Ok(Some(evt)) => {
                    if let Event::Idle = &evt {
                        task_idle_count += 1;
                        if task_idle_count
                            > self
                                .job
                                .sealing_ctrl
                                .config()
                                .request_task_max_retries
                        {
                            tracing::info!(
                            "The task has returned `Event::Idle` for more than {} times. break the task",
                            self.job.sealing_ctrl.config().request_task_max_retries
                        );

                            // when the planner tries to request a task but fails(including no task) for more than
                            // `config::sealing::request_task_max_retries` times, this task is really considered idle,
                            // break this task loop. that we have a chance to reload `sealing_thread` hot config file,
                            // or do something else.

                            if self.job.sealing_ctrl.config().check_modified() {
                                // cleanup sector if the hot config modified
                                self.job.finalize()?;
                            }
                            return Ok(R::Done);
                        }
                    }
                    event.replace(evt);
                }

                Ok(None) => match self
                    .job
                    .report_finalized(prev.range(&self.job.sectors.sectors))
                    .context("report finalized")
                {
                    Ok(_) => {
                        self.job.finalize()?;
                        return Ok(R::Done);
                    }
                    Err(terr) => self.job.retry(terr.1)?,
                },

                Err(Failure(Level::Abort, aerr)) => {
                    if let Err(rerr) = self.job.report_aborted(
                        prev.range(&self.job.sectors.sectors),
                        format!("{:#}", aerr),
                    ) {
                        tracing::error!(
                            "report aborted sector failed: {:?}",
                            rerr
                        );
                    }

                    tracing::warn!("cleanup aborted sector");
                    self.job.finalize()?;
                    return Err(aerr.abort());
                }

                Err(Failure(Level::Temporary, terr)) => self.job.retry(terr)?,

                Err(f) => return Err(f),
            }
        }
    }
}

impl SupraSealer {
    fn handle(
        &mut self,
        event: Option<Event>,
    ) -> Result<Option<Event>, Failure> {
        let prev = self.job.sectors.state.clone();

        let evnet_s = event
            .as_ref()
            .map(|e| e.to_string())
            .unwrap_or_else(|| "None".to_string());
        self.planner.num_sectors = self.job.sectors.num_sectors;

        if let Some(evt) = event {
            match &evt {
                Event::Idle | Event::Retry => {
                    tracing::debug!(
                        prev = ?self.job.sectors.state,
                        sleep = ?self.job.sealing_ctrl.config().recover_interval,
                        "Event::{:?} captured", evt
                    );

                    self.job.sealing_ctrl.wait_or_interrupted(
                        self.job.sealing_ctrl.config().recover_interval,
                    )?;
                }

                other => {
                    let next = if let Event::SetState(s) = other {
                        s.clone()
                    } else {
                        self.planner
                            .plan(other, &self.job.sectors.state)
                            .crit()?
                    };
                    self.planner
                        .apply(evt, next, &mut self.job)
                        .context("event apply")
                        .crit()?;
                    self.job.sectors.sync().context("sync sector").crit()?;
                }
            };
        };

        let span = tracing::warn_span!("handle", ?prev, current = ?self.job.sectors.state);

        let _enter = span.enter();

        self.job
            .sealing_ctrl
            .ctrl_ctx()
            .update_state(|cst| {
                cst.job.id = self.job.sectors.job_id();
                let _ =
                    cst.job.state.replace(self.job.sectors.state.to_string());
            })
            .crit()?;

        tracing::debug!("handling");

        let res = self.planner.exec(&mut self.job);

        let fail = if let Err(eref) = res.as_ref() {
            Some(SectorFailure {
                level: format!("{:?}", eref.0),
                desc: format!("{:?}", eref.1),
            })
        } else {
            None
        };

        if let Err(rerr) = self.job.report_state(
            prev,
            self.job.sectors.state.clone(),
            evnet_s,
            fail,
        ) {
            tracing::error!("report state failed: {:?}", rerr);
        }
        res
    }
}

#[derive(Default)]
pub(crate) struct SupraPlanner {
    num_sectors: usize,
}

impl PlannerTrait for SupraPlanner {
    type Job = Job;
    type State = State;
    type Event = Event;

    fn name(&self) -> &str {
        "supra"
    }

    fn plan(&self, evt: &Self::Event, st: &Self::State) -> Result<Self::State> {
        Ok(match (st, evt) {
            (State::Empty, Event::Allocate { .. }) => State::Allocated,

            (
                State::Allocated,
                Event::AcquireDeals {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::DealsAcquired { .. },
                Event::AcquireDeals {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::DealsAcquired {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::DealsAcquired { .. },
                Event::AddPiece {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::PieceAdded { .. },
                Event::AddPiece {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::PieceAdded {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::PieceAdded { .. },
                Event::BuildTreeD {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::TreeDBuilt { .. },
                Event::BuildTreeD {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::TreeDBuilt {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::TreeDBuilt { .. },
                Event::AssignTicket {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::TicketAssigned { .. },
                Event::AssignTicket {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::TicketAssigned {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (State::TicketAssigned { .. }, Event::PC1) => State::PC1Done,

            (State::PC1Done, Event::PC2(..)) => State::PC2Done,

            (
                State::PC2Done,
                Event::SubmitPC {
                    start_slot,
                    end_slot,
                },
            )
            | (
                State::PCSubmitted { .. },
                Event::SubmitPC {
                    start_slot,
                    end_slot,
                },
            ) => State::PCSubmitted {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::PCSubmitted { .. },
                Event::CheckPC {
                    start_slot,
                    end_slot,
                },
            )
            | (
                State::PCLanded { .. },
                Event::CheckPC {
                    start_slot,
                    end_slot,
                },
            ) => State::PCLanded {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (State::PCSubmitted { .. }, Event::ReSubmitPC { .. }) => {
                State::PC2Done
            }

            (
                State::PCLanded { .. },
                Event::ReSubmitPC {
                    start_slot,
                    end_slot,
                },
            ) => State::PCSubmitted {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::PCLanded { .. },
                Event::Persist {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::Persisted { .. },
                Event::Persist {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::Persisted {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::Persisted { .. },
                Event::SubmitPersistance {
                    start_slot,
                    end_slot,
                },
            )
            | (
                State::PersistanceSubmitted { .. },
                Event::SubmitPersistance {
                    start_slot,
                    end_slot,
                },
            ) => State::PersistanceSubmitted {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (
                State::PersistanceSubmitted { .. },
                Event::AssignSeed {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::SeedAssigned { .. },
                Event::AssignSeed {
                    start_slot,
                    end_slot,
                    ..
                },
            ) => State::SeedAssigned {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (State::SeedAssigned { .. }, Event::C1(_)) => State::C1Done,

            (State::C1Done { .. }, Event::C2 { slot, .. })
            | (State::C2Done { .. }, Event::C2 { slot, .. }) => {
                State::C2Done { slot: *slot }
            }

            (
                State::C2Done { .. },
                Event::SubmitProof {
                    start_slot,
                    end_slot,
                    ..
                },
            )
            | (
                State::ProofSubmitted { .. },
                Event::SubmitProof {
                    start_slot,
                    end_slot,
                },
            ) => State::ProofSubmitted {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            (State::ProofSubmitted { .. }, Event::ReSubmitProof { .. }) => {
                State::C2Done {
                    slot: self.num_sectors - 1,
                }
            }

            (
                State::Finished { .. },
                Event::ReSubmitProof {
                    start_slot,
                    end_slot,
                },
            ) => State::ProofSubmitted {
                start_slot: *start_slot - (*end_slot - *start_slot),
                end_slot: *start_slot,
            },

            (
                State::ProofSubmitted { .. },
                Event::Finish {
                    start_slot,
                    end_slot,
                },
            )
            | (
                State::Finished { .. },
                Event::Finish {
                    start_slot,
                    end_slot,
                },
            ) => State::Finished {
                start_slot: *start_slot,
                end_slot: *end_slot,
            },

            _ => {
                return Err(anyhow::anyhow!(
                    "unexpected state and event {:?} {:?}",
                    st,
                    evt
                ));
            }
        })
    }

    fn exec(
        &self,
        job: &mut Self::Job,
    ) -> Result<Option<Self::Event>, Failure> {
        let state = job.sectors.state.clone();
        let num_sectors = job.sectors.num_sectors;

        let mut inner = BatchSealer { job };

        const CHUNK_SIZE: usize = 8;
        assert_eq!(num_sectors % CHUNK_SIZE, 0);

        match state {
            State::Empty => inner.allocate(),
            State::Allocated => inner.acquire_deals(0, CHUNK_SIZE),
            State::DealsAcquired { end_slot, .. } if end_slot < num_sectors => {
                inner.acquire_deals(end_slot, end_slot + CHUNK_SIZE)
            }
            State::DealsAcquired { .. } => inner.add_pieces(0, CHUNK_SIZE),
            State::PieceAdded { end_slot, .. } if end_slot < num_sectors => {
                inner.add_pieces(end_slot, end_slot + CHUNK_SIZE)
            }
            State::PieceAdded { .. } => inner.build_tree_d(0, CHUNK_SIZE),
            State::TreeDBuilt { end_slot, .. } if end_slot < num_sectors => {
                inner.build_tree_d(end_slot, end_slot + CHUNK_SIZE)
            }
            State::TreeDBuilt { .. } => inner.assign_ticket(0, CHUNK_SIZE),
            State::TicketAssigned { end_slot, .. }
                if end_slot < num_sectors =>
            {
                inner.assign_ticket(end_slot, end_slot + CHUNK_SIZE)
            }
            State::TicketAssigned { .. } => inner.pc1(),
            State::PC1Done => inner.pc2(),
            State::PC2Done => inner.submit_pre_commit(0, CHUNK_SIZE),
            State::PCSubmitted { end_slot, .. } if end_slot < num_sectors => {
                inner.submit_pre_commit(end_slot, end_slot + CHUNK_SIZE)
            }
            State::PCSubmitted { .. } => {
                inner.check_pre_commit_state(0, CHUNK_SIZE)
            }
            State::PCLanded { end_slot, .. } if end_slot < num_sectors => {
                inner.check_pre_commit_state(end_slot, end_slot + CHUNK_SIZE)
            }
            State::PCLanded { .. } => inner.persist_sector_files(0, CHUNK_SIZE),
            State::Persisted { end_slot, .. } if end_slot < num_sectors => {
                inner.persist_sector_files(end_slot, end_slot + CHUNK_SIZE)
            }
            State::Persisted { .. } => inner.submit_persisted(0, CHUNK_SIZE),
            State::PersistanceSubmitted { end_slot, .. }
                if end_slot < num_sectors =>
            {
                inner.submit_persisted(end_slot, end_slot + CHUNK_SIZE)
            }
            State::PersistanceSubmitted { .. } => {
                inner.wait_seed(0, CHUNK_SIZE)
            }
            State::SeedAssigned { end_slot, .. } if end_slot < num_sectors => {
                inner.wait_seed(end_slot, end_slot + CHUNK_SIZE)
            }
            State::SeedAssigned { .. } => inner.commit1(),
            State::C1Done => inner.commit2(0),
            State::C2Done { slot } if slot < num_sectors - 1 => {
                inner.commit2(slot + 1)
            }
            State::C2Done { .. } => inner.submit_proof(0, CHUNK_SIZE),
            State::ProofSubmitted { end_slot, .. }
                if end_slot < num_sectors =>
            {
                inner.submit_proof(end_slot, end_slot + CHUNK_SIZE)
            }
            State::ProofSubmitted { .. } => {
                inner.check_proof_state(0, CHUNK_SIZE)
            }
            State::Finished { end_slot, .. } if end_slot < num_sectors => {
                inner.check_proof_state(end_slot, end_slot + CHUNK_SIZE)
            }
            State::Finished { .. } => return Ok(None),
            State::Aborted => return Err(TaskAborted.into()),
        }
        .map(Some)
    }

    fn apply(
        &self,
        event: Self::Event,
        state: Self::State,
        job: &mut Self::Job,
    ) -> Result<()> {
        event.apply(state, job)
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
                self.job.sectors.num_sectors as u32,
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
        let seal_proof = allocated.first().unwrap().proof_type;

        Ok(Event::Allocate {
            seal_proof,
            sectors: allocated.into_iter().map(|x| x.id).collect(),
        })
    }

    pub fn acquire_deals(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        let disable_cc = self.job.sealing_ctrl.config().disable_cc;

        if !self.job.sealing_ctrl.config().enable_deals {
            return Ok(if disable_cc {
                Event::Idle
            } else {
                Event::AcquireDeals {
                    start_slot,
                    end_slot,
                    chunk_deals: vec![None; end_slot - start_slot],
                }
            });
        }

        let spec = AcquireDealsSpec {
            max_deals: self.job.sealing_ctrl.config().max_deals,
            min_used_space: self
                .job
                .sealing_ctrl
                .config()
                .min_deal_space
                .map(|b| b.get_bytes() as usize),
        };

        let chunk_deals = (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();
                let sector = self.job.sector(slot).crit()?;

                let deals = call_rpc! {
                    self.job.rpc()=>acquire_deals(
                        sector.sector_id.clone(),
                        spec.clone(),
                    )
                }?;
                let deals_count = deals.as_ref().map(|d| d.len()).unwrap_or(0);
                tracing::debug!(
                    slot = slot,
                    count = deals_count,
                    "pieces acquired"
                );
                Ok(if disable_cc || deals_count > 0 {
                    Some(deals)
                } else {
                    None
                })
            })
            .collect::<Result<Vec<_>, Failure>>()?;

        if chunk_deals.iter().any(Option::is_none) {
            return Ok(Event::Idle);
        }
        Ok(Event::AcquireDeals {
            start_slot,
            end_slot,
            chunk_deals: chunk_deals.into_iter().map(Option::unwrap).collect(),
        })
    }

    fn add_pieces(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        let seal_proof = self.job.sectors.seal_proof;
        let chunk_pieces = (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let sector = self.job.sector(slot).crit()?;
                common::add_pieces(
                    &self.job.sealing_ctrl,
                    seal_proof,
                    path_unsealed_file(
                        self.job.store.data_path.clone(),
                        slot,
                        &sector.sector_id,
                    ),
                    sector.deals.as_ref().unwrap_or(&Vec::new()),
                )
            })
            .collect::<Result<Vec<_>, Failure>>()?;

        Ok(Event::AddPiece {
            start_slot,
            end_slot,
            chunk_pieces,
        })
    }

    fn build_tree_d(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        let chunk_comm_d = (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let sector = self.job.sector(slot).crit()?;
                common::build_tree_d(
                    &self.job.sealing_ctrl,
                    self.job.sectors.seal_proof,
                    path_cache_dir(
                        self.job.store.data_path.clone(),
                        slot,
                        &sector.sector_id,
                    ),
                    path_unsealed_file(
                        self.job.store.data_path.clone(),
                        slot,
                        &sector.sector_id,
                    ),
                    true,
                    sector.is_cc(),
                )
            })
            .collect::<Result<Vec<_>, Failure>>()?;

        Ok(Event::BuildTreeD {
            start_slot,
            end_slot,
            chunk_comm_d,
        })
    }

    fn assign_ticket(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        let chunk_ticket = (start_slot..end_slot).into_par_iter().map(|slot| {
            let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

            let sector = self.job.sector(slot).crit()?;

            let ticket = match &sector.phases.ticket {
                // Use the existing ticket when rebuilding sectors
                Some(ticket) => ticket.clone(),
                None => {
                    let ticket = call_rpc! {
                        self.job.rpc() => assign_ticket(sector.sector_id.clone(),)
                    }?;
                    tracing::debug!(ticket = ?ticket.ticket.0, epoch = ticket.epoch, "ticket assigned from sector-manager");
                    ticket
                }
            };

           Ok(ticket)
        }).collect::<Result<Vec<_>, Failure>>()?;

        Ok(Event::AssignTicket {
            start_slot,
            end_slot,
            chunk_ticket,
        })
    }

    fn pc1(&self) -> Result<Event, Failure> {
        let proof_type = self.job.sectors.seal_proof;

        let replica_ids = self
            .job
            .sectors
            .sectors
            .iter()
            .map(|s| s.phases.pc1_replica_id.context("pc1_replica_id required"))
            .collect::<Result<Vec<[u8; 32]>>>()
            .crit()?;

        self.job
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .supra_pc1
            .process(
                self.job.sealing_ctrl.ctrl_ctx(),
                SupraPC1Input {
                    block_offset: self.job.sectors.block_offset,
                    num_sectors: self.job.sectors.num_sectors,
                    registered_proof: proof_type.into(),
                    replica_ids,
                },
            )
            .crit()?;

        Ok(Event::PC1)
    }

    fn pc2(&self) -> Result<Event, Failure> {
        let data_filenames = if self.job.sector(0).crit()?.is_cc() {
            vec![]
        } else {
            (0..self.job.sectors.num_sectors)
                .map(|slot| {
                    let sector = self.job.sector(slot)?;
                    Ok(path_unsealed_file(
                        self.job.store.data_path.clone(),
                        slot,
                        &sector.sector_id,
                    )
                    .full()
                    .to_owned())
                })
                .collect::<Result<Vec<_>>>()
                .crit()?
        };

        let all_comm_r = self
            .job
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .supra_pc2
            .process(
                self.job.sealing_ctrl.ctrl_ctx(),
                SupraPC2Input {
                    registered_proof: self.job.sectors.seal_proof.into(),
                    block_offset: self.job.sectors.block_offset,
                    num_sectors: self.job.sectors.num_sectors,
                    output_dir: self.job.store.data_path.clone(),
                    data_filenames,
                },
            )
            .crit()?;
        Ok(Event::PC2(all_comm_r))
    }

    fn submit_pre_commit(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;

                let (comm_r, comm_d, ticket) =
                    if let (Some(comm_r), Some(comm_d), Some(ticket)) = (
                        sector.phases.comm_r,
                        sector.phases.comm_d,
                        sector.phases.ticket.clone(),
                    ) {
                        (comm_r, comm_d, ticket)
                    } else {
                        return Err(anyhow!(
                            "PC2 not completed. slot:{}",
                            slot
                        )
                        .crit());
                    };

                let deals = sector
                    .deals
                    .as_ref()
                    .map(|x| x.iter().map(|x| x.id).collect())
                    .unwrap_or_default();

                let pinfo = PreCommitOnChainInfo {
                    comm_r,
                    comm_d,
                    ticket,
                    deals,
                };
                let res = call_rpc! {
                    self.job.rpc() => submit_pre_commit(AllocatedSector{
                        id: sector.sector_id.clone(),
                        proof_type: self.job.sectors.seal_proof,
                    }, pinfo, sector.phases.pc2_re_submit,)
                }?;

                // TODO: handle submit reset correctly
                match res.res {
                    SubmitResult::Accepted | SubmitResult::DuplicateSubmit => {
                        Ok(())
                    }

                    SubmitResult::MismatchedSubmission => {
                        Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
                    }

                    SubmitResult::Rejected => {
                        Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
                    }

                    SubmitResult::FilesMissed => Err(anyhow!(
                    "FilesMissed should not happen for pc2 submission: {:?}",
                    res.desc
                )
                    .perm()),
                }
            })
            .collect::<Result<(), Failure>>()?;

        Ok(Event::SubmitPC {
            start_slot,
            end_slot,
        })
    }

    fn check_pre_commit_state(
        &mut self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        loop {
            let chunk_state = (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;
                if sector.phases.pc2_landed {
                    return Ok(PollPreCommitStateResp{
                        state: OnChainState::Landed,
                        desc: None,
                    })
                }
                call_rpc! {
                    self.job.rpc()=>poll_pre_commit_state(sector.sector_id.clone(), )
                }
            })
            .collect::<Result<Vec<PollPreCommitStateResp>, Failure>>()?;
            let mut landed = 0;
            for (slot, (state, sector)) in chunk_state
                .into_iter()
                .zip(self.job.sectors.sectors.iter_mut())
                .enumerate()
            {
                match state.state {
                    OnChainState::Landed => {
                        landed += 1;
                        sector.phases.pc2_landed = true;
                    }
                    OnChainState::NotFound => {
                        return Err(anyhow!(
                            "pre commit on-chain info not found: {}",
                            slot
                        )
                        .perm())
                    }

                    OnChainState::Failed => {
                        tracing::warn!(
                            slot,
                            "pre commit on-chain info failed: {}, {:?}",
                            slot,
                            state.desc
                        );
                        // TODO: make it configurable
                        self.job
                            .sealing_ctrl
                            .wait_or_interrupted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitPC {
                            start_slot,
                            end_slot,
                        });
                    }

                    OnChainState::PermFailed => {
                        return Err(anyhow!(
                        "pre commit on-chain info permanent failed: {}, {:?}",
                        slot,
                        state.desc
                    )
                        .perm())
                    }

                    OnChainState::ShouldAbort => {
                        return Err(anyhow!(
                            "pre commit info will not get on-chain: {}, {:?}",
                            slot,
                            state.desc
                        )
                        .abort())
                    }

                    OnChainState::Pending | OnChainState::Packed => {}
                }
            }

            if landed == end_slot - start_slot {
                break;
            }

            tracing::debug!(
                start_slot,
                end_slot,
                landed = landed,
                interval = ?self.job.sealing_ctrl.config().rpc_polling_interval,
                "waiting for next round of polling pre commit state",
            );

            self.job.sealing_ctrl.wait_or_interrupted(
                self.job.sealing_ctrl.config().rpc_polling_interval,
            )?;
        }

        Ok(Event::CheckPC {
            start_slot,
            end_slot,
        })
    }

    fn persist_sector_files(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        let seal_proof = self.job.sectors.seal_proof;
        let chunk_instance = (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;
                let p_sector = path_cache_dir(
                    self.job.store.data_path.clone(),
                    slot,
                    &sector.sector_id,
                );
                let p_sealed = path_sealed_file(
                    self.job.store.data_path.clone(),
                    slot,
                    &sector.sector_id,
                );
                common::persist_sector_files(
                    &self.job.sealing_ctrl,
                    seal_proof,
                    &sector.sector_id,
                    p_sector,
                    p_sealed,
                    false,
                )
            })
            .collect::<Result<Vec<_>, Failure>>()?;

        Ok(Event::Persist {
            start_slot,
            end_slot,
            chunk_instance,
        })
    }

    fn submit_persisted(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;

                cloned_required! {
                    persist_instance,
                    sector.phases.persist_instance
                }

                let checked = call_rpc! {
                    self.job.rpc() => submit_persisted_ex(sector.sector_id.clone(), persist_instance, false,)
                }?;

                if checked {
                    Ok(())
                } else {
                    Err(anyhow!("sector files are persisted but unavailable: {}", slot)).perm()
                }
            }).collect::<Result<(), Failure>>()?;
        Ok(Event::SubmitPersistance {
            start_slot,
            end_slot,
        })
    }

    fn wait_seed(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        enum WaitSeed {
            Seed(Seed),
            Delay(Duration),
        }
        let chunk_seed = loop {
            let chunk_res = (start_slot..end_slot)
                .into_par_iter()
                .map(|slot| {
                    let _rt_guard =
                        self.job.sealing_ctrl.ctx().global.rt.enter();

                    let sector = self.job.sector(slot).crit()?;

                    let wait = call_rpc! {
                        self.job.rpc()=>wait_seed(sector.sector_id.clone(), )
                    }?;

                    if let Some(seed) = wait.seed {
                        return Ok(WaitSeed::Seed(seed));
                    };

                    if !wait.should_wait || wait.delay == 0 {
                        return Err(anyhow!(
                            "invalid empty wait_seed response"
                        )
                        .temp());
                    }

                    return Ok(WaitSeed::Delay(Duration::from_secs(
                        wait.delay,
                    )));
                })
                .collect::<Result<Vec<WaitSeed>, Failure>>()?;

            let mut max_delay = None;
            for wait_seed in &chunk_res {
                match wait_seed {
                    WaitSeed::Delay(delay) if Some(delay) > max_delay => {
                        max_delay = Some(delay);
                    }
                    _ => {}
                }
            }

            match max_delay {
                Some(delay) => {
                    tracing::debug!(
                        ?delay,
                        "waiting for next round of polling seed"
                    );
                    self.job.sealing_ctrl.wait_or_interrupted(*delay)?;
                }
                None => {
                    let chunk_seed = chunk_res
                        .into_iter()
                        .map(|wait_seed| {
                            match wait_seed {
                                WaitSeed::Seed(seed) => Some(seed),
                                _ => None,
                            }
                            .unwrap()
                        })
                        .collect();
                    break chunk_seed;
                }
            }
        };

        Ok(Event::AssignSeed {
            start_slot,
            end_slot,
            chunk_seed,
        })
    }

    fn commit1(&self) -> Result<Event, Failure> {
        let registered_proof = self.job.sectors.seal_proof.into();
        let num_sectors = self.job.sectors.num_sectors;

        let all_c1out = (0..num_sectors)
            .into_par_iter()
            .map(|slot| {
                let sector = self.job.sector(slot)?;
                let p_sector = path_cache_dir(
                    self.job.store.data_path.clone(),
                    slot,
                    &sector.sector_id,
                );
                match (
                    sector.phases.pc1_replica_id,
                    sector.phases.ticket.as_ref(),
                    sector.phases.seed.as_ref(),
                ) {
                    (Some(pc1_replica_id), Some(ticket), Some(seed)) => self
                        .job
                        .sealing_ctrl
                        .ctx()
                        .global
                        .processors
                        .supra_c1
                        .process(
                            self.job.sealing_ctrl.ctrl_ctx(),
                            SupraC1Input {
                                registered_proof,
                                block_offset: self.job.sectors.block_offset,
                                num_sectors,
                                sector_slot: slot,
                                pc1_replica_id,
                                ticket: ticket.ticket.0,
                                seed: seed.seed.0,
                                cache_path: p_sector.full().clone(),
                                replica_path: p_sector.full().clone(),
                            },
                        ),
                    _ => Err(anyhow!("pc2 unfinished")),
                }
            })
            .collect::<Result<_>>()
            .crit()?;
        Ok(Event::C1(all_c1out))
    }

    fn commit2(&self, slot: usize) -> Result<Event, Failure> {
        let sector = self.job.sector(slot).crit()?;

        cloned_required! {
            c1out,
            sector.phases.c1out
        }

        let out = self
            .job
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .c2
            .process(
                self.job.sealing_ctrl.ctrl_ctx(),
                C2Input {
                    c1out,
                    prover_id: sector.prover_id,
                    sector_id: sector.sector_id.number.into(),
                    miner_id: sector.sector_id.miner,
                },
            )
            .perm()?;
        Ok(Event::C2 { slot, out })
    }

    fn submit_proof(
        &self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        (start_slot..end_slot)
            .into_par_iter()
            .map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;

                let proof = sector
                    .phases
                    .c2out
                    .clone()
                    .context("c2out required")
                    .crit()?;

                let info = ProofOnChainInfo {
                    proof: proof.proof.into(),
                };

                let res = call_rpc! {
                    self.job.rpc()=>submit_proof(sector.sector_id.clone(), info, sector.phases.c2_re_submit,)
                }?;

                // TODO: submit reset correctly
                match res.res {
                    SubmitResult::Accepted | SubmitResult::DuplicateSubmit => {
                        Ok(())
                    }

                    SubmitResult::MismatchedSubmission => {
                        Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
                    }

                    SubmitResult::Rejected => {
                        Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
                    }

                    SubmitResult::FilesMissed => Err(anyhow!(
                        "FilesMissed is not handled currently: {:?}",
                        res.desc
                    )
                    .perm()),
                }
            }).collect::<Result<(), Failure>>()?;
        Ok(Event::SubmitProof {
            start_slot,
            end_slot,
        })
    }

    fn check_proof_state(
        &mut self,
        start_slot: usize,
        end_slot: usize,
    ) -> Result<Event, Failure> {
        if self.job.sealing_ctrl.config().ignore_proof_check {
            return Ok(Event::Finish {
                start_slot,
                end_slot,
            });
        }
        loop {
            let chunk_resp = (start_slot..end_slot).into_par_iter().map(|slot| {
                let _rt_guard = self.job.sealing_ctrl.ctx().global.rt.enter();

                let sector = self.job.sector(slot).crit()?;
                if sector.phases.c2_landed {
                    return Ok(PollProofStateResp{
                        state: OnChainState::Landed,
                        desc: None,
                    })
                }
                call_rpc! {
                    self.job.rpc() => poll_proof_state(sector.sector_id.clone(),)
                }
            }).collect::<Result<Vec<PollProofStateResp>, Failure>>()?;
            let mut landed = 0;
            for (slot, (state, sector)) in chunk_resp
                .into_iter()
                .zip(self.job.sectors.sectors.iter_mut())
                .enumerate()
            {
                match state.state {
                    OnChainState::Landed => {
                        landed += 1;
                        sector.phases.c2_landed = true;
                    }
                    OnChainState::NotFound => {
                        return Err(anyhow!(
                            "proof on-chain info not found: {}",
                            slot
                        )
                        .perm())
                    }

                    OnChainState::Failed => {
                        tracing::warn!(
                            slot,
                            "proof on-chain info failed: {:?}",
                            state.desc
                        );
                        // TODO: make it configurable
                        self.job
                            .sealing_ctrl
                            .wait_or_interrupted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitProof {
                            start_slot,
                            end_slot,
                        });
                    }

                    OnChainState::PermFailed => {
                        return Err(anyhow!(
                            "proof on-chain info permanent failed: {}, {:?}",
                            slot,
                            state.desc
                        )
                        .perm())
                    }

                    OnChainState::ShouldAbort => {
                        return Err(anyhow!(
                            "sector will not get on-chain: {}, {:?}",
                            slot,
                            state.desc
                        )
                        .abort())
                    }

                    OnChainState::Pending | OnChainState::Packed => {}
                }
            }

            if landed == end_slot - start_slot {
                break;
            }

            tracing::debug!(
                landed = landed,
                interval = ?self.job.sealing_ctrl.config().rpc_polling_interval,
                "waiting for next round of polling proof state",
            );

            self.job.sealing_ctrl.wait_or_interrupted(
                self.job.sealing_ctrl.config().rpc_polling_interval,
            )?;
        }
        Ok(Event::Finish {
            start_slot,
            end_slot,
        })
    }
}

fn path_cache_dir(
    data_path: PathBuf,
    slot: usize,
    sector_id: &SectorID,
) -> Entry {
    Entry::supra_dir(
        data_path,
        PathBuf::from(format!("{:03}", slot)),
        paths::cache_dir(sector_id),
    )
}

fn path_sealed_file(
    data_path: PathBuf,
    slot: usize,
    sector_id: &SectorID,
) -> Entry {
    Entry::supra_file(
        data_path,
        PathBuf::from(format!("{:03}/{}", slot, "sealed-file")),
        paths::sealed_file(sector_id),
    )
}

fn path_unsealed_file(
    data_path: PathBuf,
    slot: usize,
    sector_id: &SectorID,
) -> Entry {
    Entry::supra_file(
        data_path,
        PathBuf::from(format!("{:03}/{}", slot, "unsealed")),
        paths::unsealed_file(sector_id),
    )
}

/// Generate the replica id as expected for Stacked DRG.
pub fn generate_replica_id<T: AsRef<[u8]>>(
    prover_id: &[u8; 32],
    sector_id: u64,
    ticket: &[u8; 32],
    comm_d: T,
    porep_seed: &[u8; 32],
) -> [u8; 32] {
    use sha2::{Digest, Sha256};

    let hash = Sha256::new()
        .chain_update(prover_id)
        .chain_update(sector_id.to_be_bytes())
        .chain_update(ticket)
        .chain_update(&comm_d)
        .chain_update(porep_seed)
        .finalize();

    bytes_into_fr_repr_safe(hash.as_ref()).into()
}

fn bytes_into_fr_repr_safe(le_bytes: &[u8]) -> [u8; 32] {
    debug_assert!(le_bytes.len() == 32);
    let mut repr = [0u8; 32];
    repr.copy_from_slice(le_bytes);
    repr[31] &= 0b0011_1111;
    repr
}
