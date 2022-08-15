use anyhow::Result;

use super::{
    super::{call_rpc, Event, State, Task},
    ExecResult, Planner,
};
use crate::logging::warn;
use crate::rpc::sealer::AllocateSectorSpec;
use crate::sealing::failure::*;

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
        unimplemented!()
    }

    fn build_tree_d_for_sealing(&self) -> ExecResult {
        unimplemented!()
    }

    fn pc1(&self) -> ExecResult {
        unimplemented!()
    }

    fn pc2(&self) -> ExecResult {
        unimplemented!()
    }

    fn check_sealed(&self) -> ExecResult {
        unimplemented!()
    }

    fn add_piece_for_snapup(&self) -> ExecResult {
        unimplemented!()
    }

    fn build_tree_d_for_snapup(&self) -> ExecResult {
        unimplemented!()
    }

    fn snap_encode(&self) -> ExecResult {
        unimplemented!()
    }

    fn snap_prove(&self) -> ExecResult {
        unimplemented!()
    }

    fn persist(&self) -> ExecResult {
        unimplemented!()
    }
}
