//! Built-in processors.
//!

use anyhow::Context;
use vc_processors::{consumer::Executor, Task};

#[cfg(feature = "builtin-add-pieces")]
pub mod piece;
#[cfg(feature = "builtin-transfer")]
mod transfer;

#[derive(Copy, Clone, Default, Debug)]
pub struct BuiltinExecutor {}

#[cfg(feature = "builtin-add-pieces")]
impl Executor<crate::tasks::AddPieces> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::AddPieces) -> Result<<crate::tasks::AddPieces as Task>::Output, Self::Error> {
        let staged_file = std::fs::OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            // to make sure that we won't write into the staged file with any data exists
            .truncate(true)
            .open(&task.staged_filepath)
            .with_context(|| format!("open staged file: {}", task.staged_filepath.display()))?;

        let mut piece_infos = Vec::with_capacity(task.pieces.len().min(1));
        for piece in task.pieces {
            tracing::debug!(piece = ?piece, "trying to add piece");
            let source = piece::fetcher::open(piece.piece_file, piece.payload_size, piece.piece_size.0).context("open piece file")?;
            let (piece_info, _) = crate::fil_proofs::write_and_preprocess(task.seal_proof_type, source, &staged_file, piece.piece_size)
                .context("add piece")?;
            piece_infos.push(piece_info);
        }

        if piece_infos.is_empty() {
            let sector_size: u64 = task.seal_proof_type.sector_size().into();

            let pi = piece::add_piece_for_cc_sector(&staged_file, sector_size).context("add piece for cc sector")?;
            piece_infos.push(pi);
        }

        Ok(piece_infos)
    }
}

#[cfg(feature = "builtin-tree-d")]
impl Executor<crate::tasks::TreeD> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::TreeD) -> Result<<crate::tasks::TreeD as Task>::Output, Self::Error> {
        crate::fil_proofs::create_tree_d(task.registered_proof, Some(task.staged_file), task.cache_dir).map(|_| true)
    }
}

#[cfg(feature = "builtin-pc1")]
impl Executor<crate::tasks::PC1> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::PC1) -> Result<<crate::tasks::PC1 as Task>::Output, Self::Error> {
        crate::fil_proofs::seal_pre_commit_phase1(
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

#[cfg(feature = "builtin-pc2")]
impl Executor<crate::tasks::PC2> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::PC2) -> Result<<crate::tasks::PC2 as Task>::Output, Self::Error> {
        crate::fil_proofs::seal_pre_commit_phase2(task.pc1out, task.cache_dir, task.sealed_file)
    }
}

#[cfg(feature = "builtin-c2")]
impl Executor<crate::tasks::C2> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::C2) -> Result<<crate::tasks::C2 as Task>::Output, Self::Error> {
        crate::fil_proofs::seal_commit_phase2(task.c1out, task.prover_id, task.sector_id)
    }
}

#[cfg(feature = "builtin-snap-encode")]
impl Executor<crate::tasks::SnapEncode> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::SnapEncode) -> Result<<crate::tasks::SnapEncode as Task>::Output, Self::Error> {
        crate::fil_proofs::snap_encode_into(
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

#[cfg(feature = "builtin-snap-prove")]
impl Executor<crate::tasks::SnapProve> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::SnapProve) -> Result<<crate::tasks::SnapProve as Task>::Output, Self::Error> {
        use crate::fil_proofs::{snap_generate_sector_update_proof, PartitionProofBytes};

        snap_generate_sector_update_proof(
            task.registered_proof,
            task.vannilla_proofs.into_iter().map(PartitionProofBytes).collect(),
            task.comm_r_old,
            task.comm_r_new,
            task.comm_d_new,
        )
    }
}

#[cfg(feature = "builtin-transfer")]
impl Executor<crate::tasks::Transfer> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::Transfer) -> Result<<crate::tasks::Transfer as Task>::Output, Self::Error> {
        task.routes.into_iter().try_for_each(|route| transfer::do_transfer(&route))?;

        Ok(true)
    }
}

#[cfg(feature = "builtin-window-post")]
impl Executor<crate::tasks::WindowPoSt> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::WindowPoSt) -> Result<<crate::tasks::WindowPoSt as Task>::Output, Self::Error> {
        use crate::{
            fil_proofs::{generate_window_post, to_prover_id, PrivateReplicaInfo},
            tasks::WindowPoStOutput,
        };
        use filecoin_proofs_api::StorageProofsError;

        let replicas = std::collections::BTreeMap::from_iter(task.replicas.into_iter().map(|rep| {
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

#[cfg(feature = "builtin-winning-post")]
impl Executor<crate::tasks::WinningPoSt> for BuiltinExecutor {
    type Error = anyhow::Error;

    fn execute(&self, task: crate::tasks::WinningPoSt) -> Result<<crate::tasks::WinningPoSt as Task>::Output, Self::Error> {
        use crate::{
            fil_proofs::{generate_winning_post, to_prover_id, PrivateReplicaInfo},
            tasks::WinningPoStOutput,
        };

        let replicas = std::collections::BTreeMap::from_iter(task.replicas.into_iter().map(|rep| {
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
