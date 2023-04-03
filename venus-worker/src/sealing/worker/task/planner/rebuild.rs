use anyhow::{anyhow, Context, Result};
use tracing::warn;

use super::{
    super::{call_rpc, field_required, Event, State, Task},
    plan, ExecResult, Planner,
};
use crate::rpc::sealer::{AllocateSectorSpec, Seed};
use crate::sealing::failure::*;

pub struct RebuildPlanner;

impl Planner for RebuildPlanner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                Event::AllocatedRebuildSector(_) => State::Allocated,
            },

            State::Allocated => {
                Event::AddPiece(_) => State::PieceAdded,
            },

            State::PieceAdded => {
                Event::BuildTreeD => State::TreeDBuilt,
            },

            State::TreeDBuilt => {
                Event::PC1(_, _) => State::PC1Done,
            },

            State::PC1Done => {
                Event::PC2(_) => State::PC2Done,
            },

            State::PC2Done => {
                Event::CheckSealed => State::SealedChecked,
            },

            State::SealedChecked => {
                Event::SkipSnap => State::SnapDone,
                Event::AddPiece(_) => State::SnapPieceAdded,
            },

            State::SnapPieceAdded => {
                Event::BuildTreeD => State::SnapTreeDBuilt,
            },

            State::SnapTreeDBuilt => {
                Event::SnapEncode(_) => State::SnapEncoded,
            },

            State::SnapEncoded => {
                Event::SnapProve(_) => State::SnapDone,
            },

            State::SnapDone => {
                Event::Persist(_) => State::Persisted,
            },

            State::Persisted => {
                Event::SubmitPersistance => State::Finished,
            },
        };

        Ok(next)
    }

    fn exec(&self, task: &mut Task<'_>) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let mut inner = Rebuild { task };

        match state {
            State::Empty => inner.empty(),

            State::Allocated => inner.add_pieces_for_sealing(),

            State::PieceAdded => inner.build_tree_d_for_sealing(),

            State::TreeDBuilt => inner.pc1(),

            State::PC1Done => inner.pc2(),

            State::PC2Done => inner.check_sealed(),

            State::SealedChecked => inner.prepare_for_snapup(),

            State::SnapPieceAdded => inner.build_tree_d_for_snapup(),

            State::SnapTreeDBuilt => inner.snap_encode(),

            State::SnapEncoded => inner.snap_prove(),

            State::SnapDone => inner.persist(),

            State::Persisted => inner.submit_persist(),

            State::Finished => return Ok(None),

            State::Aborted => {
                return Err(TaskAborted.into());
            }

            other => return Err(anyhow!("unexpected state {:?} in rebuild planner", other).abort()),
        }
        .map(From::from)
    }
}

struct Rebuild<'c, 't> {
    task: &'t mut Task<'c>,
}

impl<'c, 't> Rebuild<'c, 't> {
    fn is_snapup(&self) -> bool {
        self.task.sector.finalized.is_some()
    }

    fn empty(&self) -> ExecResult {
        let maybe_res = call_rpc! {
            self.task.ctx.global.rpc,
            allocate_rebuild_sector,
            AllocateSectorSpec {
                allowed_miners: Some(self.task.store.config.allowed_miners.clone()),
                allowed_proof_types: Some(self.task.store.config.allowed_proof_types.clone()),
            },
        };

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!(
                    "rebuild sector are not allocated yet, so we can retry even though we got the err {:?}",
                    e
                );
                return Ok(Event::Idle);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
        };

        Ok(Event::AllocatedRebuildSector(allocated))
    }

    fn add_pieces_for_sealing(&mut self) -> ExecResult {
        // if this is a snapup sector, then the deals should be used later
        let maybe_deals = if self.is_snapup() { None } else { self.task.sector.deals.clone() };

        let pieces = self.task.add_pieces(maybe_deals.unwrap_or(Vec::new()))?;

        Ok(Event::AddPiece(pieces))
    }

    fn build_tree_d_for_sealing(&mut self) -> ExecResult {
        self.task.build_tree_d(true)?;
        Ok(Event::BuildTreeD)
    }

    fn pc1(&mut self) -> ExecResult {
        let (ticket, out) = self.task.pre_commit1()?;
        Ok(Event::PC1(ticket, out))
    }

    fn pc2(&mut self) -> ExecResult {
        self.task.pre_commit2().map(Event::PC2)
    }

    fn check_sealed(&self) -> ExecResult {
        field_required! {
            ticket,
            self.task.sector.phases.ticket.as_ref()
        }

        let seed = Seed {
            seed: ticket.ticket,
            epoch: ticket.epoch,
        };

        self.task.commit1_with_seed(seed).map(|_| Event::CheckSealed)
    }

    fn prepare_for_snapup(&mut self) -> ExecResult {
        if !self.is_snapup() {
            return Ok(Event::SkipSnap);
        }

        field_required!(deals, self.task.sector.deals.clone());

        self.task.add_pieces(deals).map(Event::AddPiece)
    }

    fn build_tree_d_for_snapup(&mut self) -> ExecResult {
        self.task.build_tree_d(false).map(|_| Event::BuildTreeD)
    }

    fn snap_encode(&mut self) -> ExecResult {
        let sector_id = self.task.sector_id()?.clone();
        let proof_type = *self.task.sector_proof_type()?;

        self.task.snap_encode(sector_id, proof_type).map(Event::SnapEncode)
    }

    fn snap_prove(&mut self) -> ExecResult {
        self.task.snap_prove().map(Event::SnapProve)
    }

    fn persist(&mut self) -> ExecResult {
        let sector_id = self.task.sector_id()?;

        let (cache_dir, sealed_file) = if self.is_snapup() {
            (self.task.update_cache_dir(sector_id), self.task.update_file(sector_id))
        } else {
            (self.task.cache_dir(sector_id), self.task.sealed_file(sector_id))
        };

        self.task.persist_sector_files(cache_dir, sealed_file).map(Event::Persist)
    }

    fn submit_persist(&self) -> ExecResult {
        self.task.submit_persisted(self.is_snapup()).map(|_| Event::SubmitPersistance)
    }
}
