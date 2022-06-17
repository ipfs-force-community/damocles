//! Built-in processors.
//!

use anyhow::Result;

use super::tasks::{SnapEncode, SnapProve, Transfer, TransferRoute, TreeD, C2, PC1, PC2};
use crate::core::{Processor, Task};
use crate::fil_proofs::{
    create_tree_d, seal_commit_phase2, seal_pre_commit_phase1, seal_pre_commit_phase2, snap_encode_into, snap_generate_sector_update_proof,
    PartitionProofBytes,
};

#[derive(Copy, Clone, Default, Debug)]
pub struct BuiltinProcessor;

impl Processor<TreeD> for BuiltinProcessor {
    fn process(&self, task: TreeD) -> Result<<TreeD as Task>::Output> {
        create_tree_d(task.registered_proof, Some(task.staged_file), task.cache_dir).map(|_| true)
    }
}

impl Processor<PC1> for BuiltinProcessor {
    fn process(&self, task: PC1) -> Result<<PC1 as Task>::Output> {
        seal_pre_commit_phase1(
            task.registered_proof,
            task.cache_path,
            task.in_path,
            task.out_path,
            task.prover_id,
            task.sector_id,
            task.ticket,
            &task.piece_infos[..],
        )
    }
}

impl Processor<PC2> for BuiltinProcessor {
    fn process(&self, task: PC2) -> Result<<PC2 as Task>::Output> {
        seal_pre_commit_phase2(task.pc1out, task.cache_dir, task.sealed_file)
    }
}

impl Processor<C2> for BuiltinProcessor {
    fn process(&self, task: C2) -> Result<<C2 as Task>::Output> {
        seal_commit_phase2(task.c1out, task.prover_id, task.sector_id)
    }
}

impl Processor<SnapEncode> for BuiltinProcessor {
    fn process(&self, task: SnapEncode) -> Result<<SnapEncode as Task>::Output> {
        snap_encode_into(
            task.registered_proof,
            task.new_replica_path,
            task.new_cache_path,
            task.sector_path,
            task.sector_cache_path,
            task.staged_data_path,
            &task.piece_infos[..],
        )
    }
}

impl Processor<SnapProve> for BuiltinProcessor {
    fn process(&self, task: SnapProve) -> Result<<SnapProve as Task>::Output> {
        snap_generate_sector_update_proof(
            task.registered_proof,
            task.vannilla_proofs.into_iter().map(PartitionProofBytes).collect(),
            task.comm_r_old,
            task.comm_r_new,
            task.comm_d_new,
        )
    }
}

impl Processor<Transfer> for BuiltinProcessor {
    fn process(&self, task: Transfer) -> Result<<Transfer as Task>::Output> {
        task.routes.into_iter().try_for_each(|route| transfer::do_transfer(&route))?;

        Ok(true)
    }
}

mod transfer;
