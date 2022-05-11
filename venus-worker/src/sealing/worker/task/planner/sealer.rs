use std::fs::{create_dir_all, remove_dir_all, remove_file, OpenOptions};
use std::os::unix::fs::symlink;
use std::path::Path;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};

use super::{
    super::{call_rpc, cloned_required, field_required, Event, Stage, State, Task},
    common, plan, ExecResult, Planner,
};
use crate::logging::{debug, warn};
use crate::rpc::sealer::{AcquireDealsSpec, AllocateSectorSpec, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo, SubmitResult};
use crate::sealing::failure::*;
use crate::sealing::processor::{
    clear_cache, seal_commit_phase1, tree_d_path_in_dir, C2Input, PC1Input, PC2Input, PaddedBytesAmount, PieceInfo, UnpaddedBytesAmount,
};
use crate::sealing::util::get_all_zero_commitment;

pub struct SealerPlanner;

impl Planner for SealerPlanner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                Event::Allocate(_) => State::Allocated,
            },

            State::Allocated => {
                Event::AcquireDeals(_) => State::DealsAcquired,
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
                Event::SubmitPC => State::PCSubmitted,
            },

            State::PCSubmitted => {
                Event::ReSubmitPC => State::PC2Done,
                Event::CheckPC => State::PCLanded,
            },

            State::PCLanded => {
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

    fn exec<'t>(&self, task: &'t mut Task<'_>) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let inner = Sealer { task };
        match state {
            State::Empty => inner.handle_empty(),

            State::Allocated => inner.handle_allocated(),

            State::DealsAcquired => inner.handle_deals_acquired(),

            State::PieceAdded => inner.handle_piece_added(),

            State::TreeDBuilt => inner.handle_tree_d_built(),

            State::TicketAssigned => inner.handle_ticket_assigned(),

            State::PC1Done => inner.handle_pc1_done(),

            State::PC2Done => inner.handle_pc2_done(),

            State::PCSubmitted => inner.handle_pc_submitted(),

            State::PCLanded => inner.handle_pc_landed(),

            State::Persisted => inner.handle_persisted(),

            State::PersistanceSubmitted => inner.handle_persistance_submitted(),

            State::SeedAssigned => inner.handle_seed_assigned(),

            State::C1Done => inner.handle_c1_done(),

            State::C2Done => inner.handle_c2_done(),

            State::ProofSubmitted => inner.handle_proof_submitted(),

            State::Finished => return Ok(None),

            State::Aborted => {
                warn!("sector aborted");
                return Ok(None);
            }

            other => return Err(anyhow!("unexpected state {:?} in sealer planner", other).abort()),
        }
        .map(From::from)
    }
}

struct Sealer<'c, 't> {
    task: &'t mut Task<'c>,
}

impl<'c, 't> Sealer<'c, 't> {
    fn handle_empty(&self) -> ExecResult {
        let maybe_allocated_res = call_rpc! {
            self.task.ctx.global.rpc,
            allocate_sector,
            AllocateSectorSpec {
                allowed_miners: self.task.store.allowed_miners.as_ref().cloned(),
                allowed_proof_types: self.task.store.allowed_proof_types.as_ref().cloned(),
            },
        };

        let maybe_allocated = match maybe_allocated_res {
            Ok(a) => a,
            Err(e) => {
                warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Retry);
            }
        };

        let sector = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Retry),
        };

        // init required dirs & files
        self.task.cache_dir(&sector.id).prepare().crit()?;

        self.task.staged_file(&sector.id).prepare().crit()?;

        self.task.sealed_file(&sector.id).prepare().crit()?;

        Ok(Event::Allocate(sector))
    }

    fn handle_allocated(&self) -> ExecResult {
        if !self.task.store.config.enable_deals {
            return Ok(Event::AcquireDeals(None));
        }

        let sector_id = self.task.sector_id()?.clone();

        let deals = call_rpc! {
            self.task.ctx.global.rpc,
            acquire_deals,
            sector_id,
            AcquireDealsSpec {
                max_deals: self.task.store.config.max_deals.as_ref().cloned(),
            },
        }?;

        debug!(count = deals.as_ref().map(|d| d.len()).unwrap_or(0), "pieces acquired");

        Ok(Event::AcquireDeals(deals))
    }

    fn handle_deals_acquired(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        let mut staged_file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            // to make sure that we won't write into the staged file with any data exists
            .truncate(true)
            .open(self.task.staged_file(sector_id))
            .perm()?;

        let seal_proof_type = (*proof_type).into();

        let sector_size = proof_type.sector_size();
        let mut pieces = Vec::new();

        // acquired peices
        if let Some(deals) = self.task.sector.deals.as_ref() {
            pieces = common::add_pieces(self.task, seal_proof_type, &mut staged_file, deals)?;
        }

        if pieces.is_empty() {
            // skip AP for cc sector
            staged_file.set_len(sector_size).context("add zero commitment").perm()?;

            let commitment = get_all_zero_commitment(sector_size).context("get zero commitment").perm()?;

            let unpadded_size: UnpaddedBytesAmount = PaddedBytesAmount(sector_size).into();
            let pi = PieceInfo::new(commitment, unpadded_size).context("create piece info").perm()?;
            pieces.push(pi);
        }

        Ok(Event::AddPiece(pieces))
    }

    fn handle_piece_added(&self) -> ExecResult {
        common::build_tree_d(self.task, true)?;
        Ok(Event::BuildTreeD)
    }

    fn handle_tree_d_built(&self) -> ExecResult {
        Ok(Event::AssignTicket(None))
    }

    fn cleanup_before_pc1(&self, cache_dir: &Path, sealed_file: &Path) -> Result<()> {
        // TODO: see if we have more graceful ways to handle restarting pc1
        remove_dir_all(&cache_dir).and_then(|_| create_dir_all(&cache_dir))?;
        debug!("init cache dir {:?} before pc1", cache_dir);

        let empty_sealed_file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(true)
            .open(&sealed_file)?;

        debug!("truncate sealed file {:?} before pc1", sealed_file);
        drop(empty_sealed_file);

        Ok(())
    }

    fn handle_ticket_assigned(&self) -> ExecResult {
        let token = self.task.ctx.global.limit.acquire(Stage::PC1).crit()?;

        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        let ticket = call_rpc! {
            self.task.ctx.global.rpc,
            assign_ticket,
            sector_id.clone(),
        }?;

        debug!(ticket = ?ticket.ticket.0, epoch = ticket.epoch, "ticket assigned from sector-manager");

        field_required! {
            piece_infos,
            self.task.sector.phases.pieces.as_ref().cloned()
        }

        let cache_dir = self.task.cache_dir(sector_id);
        let staged_file = self.task.staged_file(sector_id);
        let sealed_file = self.task.sealed_file(sector_id);
        let prepared_dir = self.task.prepared_dir(sector_id);

        self.cleanup_before_pc1(cache_dir.as_ref(), sealed_file.as_ref()).crit()?;
        symlink(tree_d_path_in_dir(prepared_dir.as_ref()), tree_d_path_in_dir(cache_dir.as_ref())).crit()?;

        field_required! {
            prove_input,
            self.task.sector.base.as_ref().map(|b| b.prove_input)
        }

        let out = self
            .task
            .ctx
            .global
            .processors
            .pc1
            .process(PC1Input {
                registered_proof: (*proof_type).into(),
                cache_path: cache_dir.into(),
                in_path: staged_file.into(),
                out_path: sealed_file.into(),
                prover_id: prove_input.0,
                sector_id: prove_input.1,
                ticket: ticket.ticket.0,
                piece_infos,
            })
            .perm()?;

        drop(token);
        Ok(Event::PC1(ticket, out))
    }

    fn cleanup_before_pc2(&self, cache_dir: &Path) -> Result<()> {
        for entry_res in cache_dir.read_dir()? {
            let entry = entry_res?;
            let fname = entry.file_name();
            if let Some(fname_str) = fname.to_str() {
                let should =
                    fname_str == "p_aux" || fname_str == "t_aux" || fname_str.contains("tree-c") || fname_str.contains("tree-r-last");

                if !should {
                    continue;
                }

                let p = entry.path();
                remove_file(&p).with_context(|| format!("remove cached file {:?}", p))?;
                debug!("remove cached file {:?} before pc2", p);
            }
        }

        Ok(())
    }

    fn handle_pc1_done(&self) -> ExecResult {
        let token = self.task.ctx.global.limit.acquire(Stage::PC2).crit()?;

        let sector_id = self.task.sector_id()?;

        field_required! {
            pc1out,
            self.task.sector.phases.pc1out.as_ref().cloned()
        }

        let cache_dir = self.task.cache_dir(sector_id);
        let sealed_file = self.task.sealed_file(sector_id);

        self.cleanup_before_pc2(cache_dir.as_ref()).crit()?;

        let out = self
            .task
            .ctx
            .global
            .processors
            .pc2
            .process(PC2Input {
                pc1out,
                cache_dir: cache_dir.into(),
                sealed_file: sealed_file.into(),
            })
            .perm()?;

        drop(token);
        Ok(Event::PC2(out))
    }

    fn handle_pc2_done(&self) -> ExecResult {
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

        let deals = self
            .task
            .sector
            .deals
            .as_ref()
            .map(|d| d.iter().map(|i| i.id).collect())
            .unwrap_or_default();

        let pinfo = PreCommitOnChainInfo {
            comm_r,
            comm_d,
            ticket,
            deals,
        };

        let res = call_rpc! {
            self.task.ctx.global.rpc,
            submit_pre_commit,
            sector,
            pinfo,
            self.task.sector.phases.pc2_re_submit,
        }?;

        // TODO: handle submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitPC),

            SubmitResult::MismatchedSubmission => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort()),

            SubmitResult::FilesMissed => Err(anyhow!("FilesMissed should not happen for pc2 submission: {:?}", res.desc).perm()),
        }
    }

    fn handle_pc_submitted(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;

        'POLL: loop {
            let state = call_rpc! {
                self.task.ctx.global.rpc,
                poll_pre_commit_state,
                sector_id.clone(),
            }?;

            match state.state {
                OnChainState::Landed => break 'POLL,
                OnChainState::NotFound => return Err(anyhow!("pre commit on-chain info not found").perm()),

                OnChainState::Failed => {
                    warn!("pre commit on-chain info failed: {:?}", state.desc);
                    // TODO: make it configurable
                    self.task.wait_or_interruptted(Duration::from_secs(30))?;
                    return Ok(Event::ReSubmitPC);
                }

                OnChainState::PermFailed => return Err(anyhow!("pre commit on-chain info permanent failed: {:?}", state.desc).perm()),

                OnChainState::ShouldAbort => return Err(anyhow!("pre commit info will not get on-chain: {:?}", state.desc).abort()),

                OnChainState::Pending | OnChainState::Packed => {}
            }

            debug!(
                state = ?state.state,
                interval = ?self.task.store.config.rpc_polling_interval,
                "waiting for next round of polling pre commit state",
            );

            self.task.wait_or_interruptted(self.task.store.config.rpc_polling_interval)?;
        }

        debug!("pre commit landed");

        Ok(Event::CheckPC)
    }

    fn handle_pc_landed(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        let cache_dir = self.task.cache_dir(sector_id);
        let sealed_file = self.task.sealed_file(sector_id);

        let ins_name = common::persist_sector_files(self.task, cache_dir, sealed_file)?;

        Ok(Event::Persist(ins_name))
    }

    fn handle_persisted(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;

        field_required! {
            instance,
            self.task.sector.phases.persist_instance.as_ref().cloned()
        }

        let checked = call_rpc! {
            self.task.ctx.global.rpc,
            submit_persisted,
            sector_id.clone(),
            instance,
        }?;

        if checked {
            Ok(Event::SubmitPersistance)
        } else {
            Err(anyhow!("sector files are persisted but unavailable for sealer")).perm()
        }
    }

    fn handle_persistance_submitted(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;

        let seed = loop {
            let wait = call_rpc! {
                self.task.ctx.global.rpc,
                wait_seed,
                sector_id.clone(),
            }?;

            if let Some(seed) = wait.seed {
                break seed;
            };

            if !wait.should_wait || wait.delay == 0 {
                return Err(anyhow!("invalid empty wait_seed response").temp());
            }

            let delay = Duration::from_secs(wait.delay);

            debug!(?delay, "waiting for next round of polling seed");

            self.task.wait_or_interruptted(delay)?;
        };

        Ok(Event::AssignSeed(seed))
    }

    fn handle_seed_assigned(&self) -> ExecResult {
        let token = self.task.ctx.global.limit.acquire(Stage::C1).crit()?;

        let sector_id = self.task.sector_id()?;

        field_required! {
            prove_input,
            self.task.sector.base.as_ref().map(|b| b.prove_input)
        }

        let (seal_prover_id, seal_sector_id) = prove_input;

        field_required! {
            piece_infos,
            self.task.sector.phases.pieces.as_ref()
        }

        field_required! {
            ticket,
            self.task.sector.phases.ticket.as_ref()
        }

        cloned_required! {
            seed,
            self.task.sector.phases.seed
        }

        cloned_required! {
            p2out,
            self.task.sector.phases.pc2out
        }

        let cache_dir = self.task.cache_dir(sector_id);
        let sealed_file = self.task.sealed_file(sector_id);

        let out = seal_commit_phase1(
            cache_dir.into(),
            sealed_file.into(),
            seal_prover_id,
            seal_sector_id,
            ticket.ticket.0,
            seed.seed.0,
            p2out,
            piece_infos,
        )
        .perm()?;

        drop(token);
        Ok(Event::C1(out))
    }

    fn handle_c1_done(&self) -> ExecResult {
        let token = self.task.ctx.global.limit.acquire(Stage::C2).crit()?;

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
            .ctx
            .global
            .processors
            .c2
            .process(C2Input {
                c1out,
                prover_id,
                sector_id,
                miner_id,
            })
            .perm()?;

        drop(token);
        Ok(Event::C2(out))
    }

    fn handle_c2_done(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?.clone();

        cloned_required! {
            proof,
            self.task.sector.phases.c2out
        }

        let info = ProofOnChainInfo { proof: proof.proof.into() };

        let res = call_rpc! {
            self.task.ctx.global.rpc,
            submit_proof,
            sector_id,
            info,
            self.task.sector.phases.c2_re_submit,
        }?;

        // TODO: submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitProof),

            SubmitResult::MismatchedSubmission => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort()),

            SubmitResult::FilesMissed => Err(anyhow!("FilesMissed is not handled currently: {:?}", res.desc).perm()),
        }
    }

    fn handle_proof_submitted(&self) -> ExecResult {
        field_required! {
            allocated,
            self.task.sector.base.as_ref().map(|b| &b.allocated)
        }

        let sector_id = &allocated.id;

        if !self.task.store.config.ignore_proof_check {
            'POLL: loop {
                let state = call_rpc! {
                    self.task.ctx.global.rpc,
                    poll_proof_state,
                    sector_id.clone(),
                }?;

                match state.state {
                    OnChainState::Landed => break 'POLL,
                    OnChainState::NotFound => return Err(anyhow!("proof on-chain info not found").perm()),

                    OnChainState::Failed => {
                        warn!("proof on-chain info failed: {:?}", state.desc);
                        // TODO: make it configurable
                        self.task.wait_or_interruptted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitProof);
                    }

                    OnChainState::PermFailed => return Err(anyhow!("proof on-chain info permanent failed: {:?}", state.desc).perm()),

                    OnChainState::ShouldAbort => return Err(anyhow!("sector will not get on-chain: {:?}", state.desc).abort()),

                    OnChainState::Pending | OnChainState::Packed => {}
                }

                debug!(
                    state = ?state.state,
                    interval = ?self.task.store.config.rpc_polling_interval,
                    "waiting for next round of polling proof state",
                );

                self.task.wait_or_interruptted(self.task.store.config.rpc_polling_interval)?;
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
