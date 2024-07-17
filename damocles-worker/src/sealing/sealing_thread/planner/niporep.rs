use std::{path::PathBuf, time::Duration};

use super::{
    super::{call_rpc, cloned_required, field_required},
    common::{self, event::Event, sector::State, task::Task},
    plan, PlannerTrait, PLANNER_NAME_NIPOREP,
};
use crate::logging::{debug, warn, info};
use crate::rpc::sealer::{
    AcquireDealsSpec, AllocateSectorSpec, OnChainState, PreCommitOnChainInfo,
    ProofOnChainInfo, Randomness, Seed, SubmitResult,
};
use crate::sealing::failure::*;
use crate::sealing::processor::{
    clear_cache, clear_layer_data, generate_synth_proofs, seal_commit_phase1,
    ApiFeature, C2Input, RegisteredSealProof,
};
use anyhow::{anyhow, Context, Result};
use fil_clock::ChainEpoch;
use vc_processors::b64serde::{BytesArray32, BytesVec};
use vc_processors::builtin::tasks::{STAGE_NAME_C1, STAGE_NAME_C2};

#[derive(Default)]
pub(crate) struct NiPoRepPlanner;

impl PlannerTrait for NiPoRepPlanner {
    type Job = Task;
    type State = State;
    type Event = Event;

    fn name(&self) -> &str {
        PLANNER_NAME_NIPOREP
    }

    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                Event::Allocate(_) => State::Allocated,
            },

            State::Allocated => {
                // As niporep limit to cc sector, will not assign deal at this step
                Event::AcquireDealsV2(_) => State::DealsAcquired,
            },

            State::DealsAcquired => {
                Event::AddPiece(_) => State::PieceAdded,
            },

            State::PieceAdded => {
                Event::BuildTreeD => State::TreeDBuilt,
            },

            State::TreeDBuilt => {
                Event::AssignTicket(_) => State::TicketAssigned,
            },

            State::TicketAssigned => {
                Event::PC1(_, _) => State::PC1Done,
            },

            State::PC1Done => {
                Event::PC2(_) => State::PC2Done,
            },

            State::PC2Done => {
                Event::Persist(_) => State::Persisted,
            },

            State::Persisted => {
                Event::SubmitPersistance => State::PersistanceSubmitted,
            },

            State::PersistanceSubmitted => {
                Event::AssignSeed(_) => State::SeedAssigned,
            },

            State::SeedAssigned => {
                Event::C1(_) => State::C1Done,
            },

            State::C1Done => {
                Event::C2(_) => State::C2Done,
            },

            State::C2Done => {
                Event::SubmitProof => State::ProofSubmitted,
            },

            State::ProofSubmitted => {
                Event::ReSubmitProof => State::C2Done,
                Event::Finish => State::Finished,
            },
        };

        Ok(next)
    }

    fn exec(&self, task: &mut Task) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let inner = NiPoRep { task };
        match state {
            State::Empty => inner.handle_empty(),

            State::Allocated => inner.handle_allocated(),
            // As niporep limit to cc sector, will not assign deal at this step
            State::DealsAcquired => inner.handle_deals_acquired(),

            State::PieceAdded => inner.handle_piece_added(),

            State::TreeDBuilt => inner.handle_tree_d_built(),

            State::TicketAssigned => inner.handle_ticket_assigned(),

            State::PC1Done => inner.handle_pc1_done(),

            State::PC2Done => inner.handle_pc2_done(),

            State::Persisted => inner.handle_persisted(),

            State::PersistanceSubmitted => inner.handle_persistance_submitted(),

            State::SeedAssigned => inner.handle_seed_assigned(),

            State::C1Done => inner.handle_c1_done(),

            State::C2Done => inner.handle_c2_done(),

            State::ProofSubmitted => inner.handle_proof_submitted(),

            State::Finished => return Ok(None),

            State::Aborted => {
                return Err(TaskAborted.into());
            }

            other => {
                return Err(anyhow!(
                    "unexpected state {:?} in sealer planner",
                    other
                )
                .abort())
            }
        }
        .map(Some)
    }

    fn apply(&self, event: Event, state: State, task: &mut Task) -> Result<()> {
        event.apply(state, task)
    }
}

struct NiPoRep<'t> {
    task: &'t mut Task,
}

impl<'t> NiPoRep<'t> {
    fn handle_empty(&self) -> Result<Event, Failure> {
        let maybe_allocated_res = call_rpc! {
            self.task.rpc()=>allocate_sector(AllocateSectorSpec {
                allowed_miners: Some(self.task.sealing_ctrl.config().allowed_miners.clone()),
                allowed_proof_types: Some(self.task.sealing_ctrl.config().allowed_proof_types.clone()),
            },true,)
        };

        let maybe_allocated = match maybe_allocated_res {
            Ok(a) => a,
            Err(e) => {
                warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Idle);
            }
        };

        let sector = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
        };

        // init required dirs & files
        self.task.cache_dir(&sector.id).prepare().crit()?;

        self.task.staged_file(&sector.id).prepare().crit()?;

        self.task.sealed_file(&sector.id).prepare().crit()?;

        Ok(Event::Allocate(sector))
    }

    fn handle_allocated(&self) -> Result<Event, Failure> {
        Ok(Event::AcquireDealsV2(None))
    }

    fn handle_deals_acquired(&self) -> Result<Event, Failure> {
        let deals = self.task.sector.deals();
        let pieces = common::add_pieces(self.task, &deals)?;

        Ok(Event::AddPiece(pieces))
    }

    fn handle_piece_added(&self) -> Result<Event, Failure> {
        common::build_tree_d(self.task, true)?;
        info!("[ni] treed build");
        Ok(Event::BuildTreeD)
    }

    fn handle_tree_d_built(&self) -> Result<Event, Failure> {
        Ok(Event::AssignTicket(None))
    }

    fn handle_ticket_assigned(&self) -> Result<Event, Failure> {
        let (ticket, out) = common::pre_commit1(self.task)?;
        Ok(Event::PC1(ticket, out))
    }

    fn handle_pc1_done(&self) -> Result<Event, Failure> {
        let verify_after_pc2 = self.task.sealing_ctrl.config().verify_after_pc2;
        common::pre_commit2(self.task, verify_after_pc2).map(Event::PC2)
    }

    fn handle_pc2_done(&self) -> Result<Event, Failure> {
        field_required! {
            sector,
            self.task.sector.base.as_ref().map(|b| b.allocated.clone())
        }

        field_required! {
            comm_r,
            self.task.sector.phases.pc2out.as_ref().map(|out| out.comm_r)
        }

        field_required! {
            comm_d,
            self.task.sector.phases.pc2out.as_ref().map(|out| out.comm_d)
        }

        field_required! {
            ticket,
            self.task.sector.phases.ticket.as_ref().cloned()
        }

        let pinfo = PreCommitOnChainInfo {
            comm_r,
            comm_d,
            ticket,
        };

        let res = call_rpc! {
            self.task.rpc() => submit_pre_commit(sector, pinfo, self.task.sector.phases.pc2_re_submit,)
        }?;

        // TODO: handle submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => {
                info!("pre_commit info accepted");
            }

            SubmitResult::MismatchedSubmission => {
                return Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
            }

            SubmitResult::Rejected => {
                return Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
            }

            SubmitResult::FilesMissed => {
                return Err(anyhow!(
                "FilesMissed should not happen for pc2 submission: {:?}",
                res.desc).perm())
            }
        };
        let sector_id = self.task.sector_id()?;
        let cache_dir = self.task.cache_dir(sector_id);
        let sealed_file = self.task.sealed_file(sector_id);

        let ins_name =
            common::persist_sector_files(self.task, cache_dir, sealed_file)?;

        Ok(Event::Persist(ins_name))
    }

    fn handle_persisted(&self) -> Result<Event, Failure> {
        info!("[ni] persisted");
        common::submit_persisted(self.task, false)
            .map(|_| Event::SubmitPersistance)
    }

    fn handle_persistance_submitted(&self) -> Result<Event, Failure> {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?.to_ni_porep();

        let seed = loop {
            let wait = call_rpc! {
                self.task.rpc()=>wait_seed(sector_id.clone(), proof_type,)
            }?;

            if let Some(seed) = wait.seed {
                break seed;
            };

            if !wait.should_wait || wait.delay == 0 {
                return Err(anyhow!("invalid empty wait_seed response").temp());
            }

            let delay = Duration::from_secs(wait.delay);

            debug!(?delay, "waiting for next round of polling seed");

            self.task.sealing_ctrl.wait_or_interrupted(delay)?;
        };

        Ok(Event::AssignSeed(seed))
    }

    fn handle_seed_assigned(&self) -> Result<Event, Failure> {
        cloned_required! {
            seed,
            self.task.sector.phases.seed
        }
        info!(?seed, "[ni] seed");
        common::commit1_with_seed(self.task, seed).map(Event::C1)
    }

    fn handle_c1_done(&self) -> Result<Event, Failure> {
        info!("[ni] c1 done");
        let _token = self
            .task
            .sealing_ctrl
            .ctrl_ctx()
            .wait(STAGE_NAME_C2)
            .crit()?;

        let miner_id = self.task.sector_id()?.miner;

        cloned_required! {
            c1out,
            self.task.sector.phases.c1out
        }

        cloned_required! {
            prove_input,
            self.task.sector.base.as_ref().map(|b| b.prove_input)
        }

        let (prover_id, sector_id) = prove_input;

        let out = self
            .task
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .c2
            .process(
                self.task.sealing_ctrl.ctrl_ctx(),
                C2Input {
                    c1out,
                    prover_id,
                    sector_id,
                    miner_id,
                },
            )
            .perm()?;
        Ok(Event::C2(out))
    }

    fn handle_c2_done(&self) -> Result<Event, Failure> {
        info!("[ni] c2 done");
        let sector_id = self.task.sector_id()?.clone();

        cloned_required! {
            proof,
            self.task.sector.phases.c2out
        }

        let info = ProofOnChainInfo {
            proof: proof.proof.into(),
        };

        let res = call_rpc! {
            self.task.rpc()=>submit_proof(sector_id, info, self.task.sector.phases.c2_re_submit,)
        }?;
        info!(?res.res, "[ni] submit proof res");
        info!(?res.desc, "[ni] submit proof desc");
        // TODO: submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => {
                info!("[ni] event submit proof");
                Ok(Event::SubmitProof)
            }

            SubmitResult::MismatchedSubmission => {
                info!("[ni] sr err 1");
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
            }

            SubmitResult::Rejected => {
                info!("[ni] sr err 2");
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
            }

            SubmitResult::FilesMissed => {
                info!("[ni] sr err 3");
                Err(anyhow!(
                    "FilesMissed is not handled currently: {:?}",
                    res.desc
                ).perm())
            }
        }
    }

    fn handle_proof_submitted(&self) -> Result<Event, Failure> {
        info!("[ni] proof submitted");
        field_required! {
            allocated,
            self.task.sector.base.as_ref().map(|b| &b.allocated)
        }

        let sector_id = &allocated.id;

        if !self.task.sealing_ctrl.config().ignore_proof_check {
            'POLL: loop {
                let state = call_rpc! {
                    self.task.rpc() => poll_proof_state(sector_id.clone(),)
                }?;

                match state.state {
                    OnChainState::Landed => break 'POLL,
                    OnChainState::NotFound => {
                        return Err(
                            anyhow!("proof on-chain info not found").perm()
                        )
                    }

                    OnChainState::Failed => {
                        warn!("proof on-chain info failed: {:?}", state.desc);
                        // TODO: make it configurable
                        self.task
                            .sealing_ctrl
                            .wait_or_interrupted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitProof);
                    }

                    OnChainState::PermFailed => {
                        return Err(anyhow!(
                            "proof on-chain info permanent failed: {:?}",
                            state.desc
                        )
                        .perm())
                    }

                    OnChainState::ShouldAbort => {
                        return Err(anyhow!(
                            "sector will not get on-chain: {:?}",
                            state.desc
                        )
                        .abort())
                    }

                    OnChainState::Pending | OnChainState::Packed => {}
                }

                debug!(
                    state = ?state.state,
                    interval = ?self.task.sealing_ctrl.config().rpc_polling_interval,
                    "waiting for next round of polling proof state",
                );

                self.task.sealing_ctrl.wait_or_interrupted(
                    self.task.sealing_ctrl.config().rpc_polling_interval,
                )?;
            }
        }

        let cache_dir = self.task.cache_dir(sector_id);
        let sector_size = allocated.proof_type.sector_size();

        // we should be careful here, use failure as temporary
        clear_cache(sector_size, cache_dir.as_ref()).temp()?;
        debug!(
            dir = ?&cache_dir,
            "clean up unnecessary cached files"
        );

        Ok(Event::Finish)
    }
}
