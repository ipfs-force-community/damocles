use anyhow::{Context, Result};

use super::{
    super::{call_rpc, field_required, Event, State, Task},
    common, ExecResult, Planner,
};
use crate::logging::warn;
use crate::rpc::sealer::{AllocateSectorSpec, Seed};
use crate::sealing::failure::*;
use crate::sealing::processor::STAGE_NAME_SNAP_ENCODE;

pub struct RebuildPlanner;

impl Planner for RebuildPlanner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        unimplemented!()
    }

    fn exec<'t>(&self, task: &'t mut Task<'_>) -> Result<Option<Event>, Failure> {
        unimplemented!()
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
                allowed_miners: self.task.store.allowed_miners.as_ref().cloned(),
                allowed_proof_types: self.task.store.allowed_proof_types.as_ref().cloned(),
            },
        };

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!(
                    "rebuild sector are not allocated yet, so we can retry even though we got the err {:?}",
                    e
                );
                return Ok(Event::Retry);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Retry),
        };

        Ok(Event::AllocatedRebuildSector(allocated))
    }

    fn add_pieces_for_sealing(&self) -> ExecResult {
        // if this is a snapup sector, then the deals should be used later
        let maybe_deals = if self.is_snapup() { None } else { self.task.sector.deals.as_ref() };

        let pieces = common::maybe_add_pieces(self.task, maybe_deals)?;

        Ok(Event::AddPiece(pieces))
    }

    fn build_tree_d_for_sealing(&self) -> ExecResult {
        common::build_tree_d(self.task, true)?;
        Ok(Event::BuildTreeD)
    }

    fn pc1(&self) -> ExecResult {
        let (ticket, out) = common::pre_commit1(self.task)?;
        Ok(Event::PC1(ticket, out))
    }

    fn pc2(&self) -> ExecResult {
        common::pre_commit2(self.task).map(Event::PC2)
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

        common::commit1_with_seed(self.task, seed).map(|_| Event::CheckSealed)
    }

    fn add_piece_for_snapup(&self) -> ExecResult {
        if !self.is_snapup() {
            return Ok(Event::SkipSnap);
        }

        field_required!(deals, self.task.sector.deals.as_ref());

        common::maybe_add_pieces(self.task, Some(deals)).map(Event::AddPiece)
    }

    fn build_tree_d_for_snapup(&self) -> ExecResult {
        common::build_tree_d(self.task, false).map(|_| Event::BuildTreeD)
    }

    fn snap_encode(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        common::snap_encode(self.task, sector_id, proof_type).map(Event::SnapEncode)
    }

    fn snap_prove(&self) -> ExecResult {
        common::snap_prove(self.task).map(Event::SnapProve)
    }

    fn persist(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;

        let (cache_dir, sealed_file) = if self.is_snapup() {
            (self.task.update_cache_dir(sector_id), self.task.update_file(sector_id))
        } else {
            (self.task.cache_dir(sector_id), self.task.sealed_file(sector_id))
        };

        common::persist_sector_files(self.task, cache_dir, sealed_file).map(Event::Persist)
    }

    fn submit_persist(&self) {}
}
