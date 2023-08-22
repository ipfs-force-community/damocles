//! Built-in processors.
//!

use std::collections::BTreeMap;
use std::fs;

use anyhow::{Context, Result};
use filecoin_proofs_api::StorageProofsError;
use tracing::debug;

use super::tasks::{
    AddPieces, SnapEncode, SnapProve, Transfer, TransferRoute, TreeD, Unseal, WindowPoSt, WindowPoStOutput, WinningPoSt, WinningPoStOutput,
    C2, PC1, PC2,
};
use crate::core::{Processor, Task};
use crate::fil_proofs::{
    create_tree_d, generate_window_post, generate_winning_post, seal_commit_phase2, seal_pre_commit_phase1, seal_pre_commit_phase2,
    snap_encode_into, snap_generate_sector_update_proof, to_prover_id, unseal_range, write_and_preprocess, PartitionProofBytes,
    PrivateReplicaInfo,
};

pub mod piece;
mod transfer;

#[derive(Copy, Clone, Default, Debug)]
pub struct BuiltinProcessor;

impl Processor<AddPieces> for BuiltinProcessor {
    fn process(&self, task: AddPieces) -> Result<<AddPieces as Task>::Output> {
        let staged_file = fs::OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            // to make sure that we won't write into the staged file with any data exists
            .truncate(true)
            .open(&task.staged_filepath)
            .with_context(|| format!("open staged file: {}", task.staged_filepath.display()))?;

        let mut piece_infos = Vec::with_capacity(task.pieces.len().min(1));
        for piece in task.pieces {
            debug!(piece = ?piece, "trying to add piece");
            let source = piece::fetcher::open(piece.piece_file, piece.payload_size, piece.piece_size.0).context("open piece file")?;
            let (piece_info, _) =
                write_and_preprocess(task.seal_proof_type, source, &staged_file, piece.piece_size).context("add piece")?;
            piece_infos.push(piece_info);
        }

        if piece_infos.is_empty() {
            let sector_size: u64 = task.seal_proof_type.sector_size().into();

            let pi = piece::add_piece_for_cc_sector(&staged_file, sector_size).context("add piece for cc secrtor")?;
            piece_infos.push(pi);
        }

        Ok(piece_infos)
    }
}

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

impl Processor<WindowPoSt> for BuiltinProcessor {
    fn process(&self, task: WindowPoSt) -> Result<<WindowPoSt as Task>::Output> {
        let replicas = BTreeMap::from_iter(task.replicas.into_iter().map(|rep| {
            (
                rep.sector_id,
                PrivateReplicaInfo::new(task.proof_type, rep.comm_r, rep.cache_dir, rep.sealed_file),
            )
        }));

        generate_window_post(&task.seed, &replicas, to_prover_id(task.miner_id))
            .map(|proofs| WindowPoStOutput {
                proofs: proofs.into_iter().map(|r| r.1).collect(),
                faults: vec![],
            })
            .or_else(|e| {
                if let Some(StorageProofsError::FaultySectors(sectors)) = e.downcast_ref::<StorageProofsError>() {
                    return Ok(WindowPoStOutput {
                        proofs: vec![],
                        faults: sectors.iter().map(|id| (*id).into()).collect(),
                    });
                }

                Err(e)
            })
    }
}

impl Processor<WinningPoSt> for BuiltinProcessor {
    fn process(&self, task: WinningPoSt) -> Result<<WinningPoSt as Task>::Output> {
        let replicas = BTreeMap::from_iter(task.replicas.into_iter().map(|rep| {
            (
                rep.sector_id,
                PrivateReplicaInfo::new(task.proof_type, rep.comm_r, rep.cache_dir, rep.sealed_file),
            )
        }));

        generate_winning_post(&task.seed, &replicas, to_prover_id(task.miner_id)).map(|proofs| WinningPoStOutput {
            proofs: proofs.into_iter().map(|r| r.1).collect(),
        })
    }
}

impl Processor<Unseal> for BuiltinProcessor {
    fn process(&self, task: Unseal) -> Result<<Unseal as Task>::Output> {
        let unsealed_output = fs::OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(true)
            .open(&task.unsealed_output)
            .with_context(|| format!("open unsealed file: {}", task.unsealed_output.display()))?;

        unseal_range(
            task.registered_proof,
            &task.cache_dir,
            &task.sealed_file,
            unsealed_output,
            task.prover_id,
            task.sector_id,
            task.comm_d,
            task.ticket,
            task.offset,
            task.num_bytes,
        )
    }
}

#[derive(Copy, Clone, Default, Debug)]
pub struct TransferProcessor {
    disable_link: bool,
}

impl TransferProcessor {
    pub fn new(disable_link: bool) -> Self {
        Self { disable_link }
    }
}

impl Processor<Transfer> for TransferProcessor {
    fn process(&self, task: Transfer) -> Result<<Transfer as Task>::Output> {
        task.routes
            .into_iter()
            .try_for_each(|route| transfer::do_transfer_inner(&route, self.disable_link))?;

        Ok(true)
    }
}
