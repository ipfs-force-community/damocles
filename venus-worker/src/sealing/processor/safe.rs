use std::panic::catch_unwind;
use std::path::PathBuf;

use anyhow::Result;
use filecoin_proofs::StoreConfig;
pub use filecoin_proofs::{EmptySectorUpdateEncoded, EmptySectorUpdateProof};
use filecoin_proofs_api::seal;
pub use filecoin_proofs_api::seal::{
    clear_cache, write_and_preprocess, Labels, SealCommitPhase1Output, SealCommitPhase2Output,
    SealPreCommitPhase1Output, SealPreCommitPhase2Output,
};
pub use filecoin_proofs_api::update::{
    empty_sector_update_encode_into, generate_empty_sector_update_proof_with_vanilla,
    generate_partition_proofs, verify_empty_sector_update_proof, verify_partition_proofs,
};
pub use filecoin_proofs_api::{
    Commitment, PaddedBytesAmount, PartitionProofBytes, PieceInfo, ProverId, RegisteredSealProof,
    RegisteredUpdateProof, SectorId, Ticket, UnpaddedBytesAmount,
};
use storage_proofs_core::cache_key::CacheKey;

use super::proof;

macro_rules! safe_call {
    ($ex:expr) => {
        match catch_unwind(move || $ex.map_err(|e| format!("{:?}", e))) {
            Ok(r) => r.map_err(anyhow::Error::msg),
            Err(p) => {
                let error_msg = match p.downcast_ref::<&'static str>() {
                    Some(message) => message.to_string(),
                    _ => format!("non-str unwind err: {:?}", p),
                };

                Err(anyhow::Error::msg(error_msg))
            }
        }
    };
}

pub fn seal_commit_phase1(
    cache_path: PathBuf,
    replica_path: PathBuf,
    prover_id: ProverId,
    sector_id: SectorId,
    ticket: Ticket,
    seed: Ticket,
    pre_commit: SealPreCommitPhase2Output,
    piece_infos: &[PieceInfo],
) -> Result<SealCommitPhase1Output> {
    safe_call! {
        seal::seal_commit_phase1(
            cache_path,
            replica_path,
            prover_id,
            sector_id,
            ticket,
            seed,
            pre_commit,
            piece_infos,
        )
    }
}

pub fn seal_commit_phase2(
    phase1_output: SealCommitPhase1Output,
    prover_id: ProverId,
    sector_id: SectorId,
) -> Result<SealCommitPhase2Output> {
    safe_call! {
        seal::seal_commit_phase2(phase1_output, prover_id, sector_id)
    }
}

pub fn seal_pre_commit_phase1(
    registered_proof: RegisteredSealProof,
    cache_path: PathBuf,
    in_path: PathBuf,
    out_path: PathBuf,
    prover_id: ProverId,
    sector_id: SectorId,
    ticket: Ticket,
    piece_infos: &[PieceInfo],
) -> Result<SealPreCommitPhase1Output> {
    safe_call! {
        seal::seal_pre_commit_phase1(
            registered_proof,
            cache_path,
            in_path,
            out_path,
            prover_id,
            sector_id,
            ticket,
            piece_infos,
        )
    }
}

pub fn seal_pre_commit_phase2(
    phase1_output: SealPreCommitPhase1Output,
    cache_path: PathBuf,
    out_path: PathBuf,
) -> Result<SealPreCommitPhase2Output> {
    safe_call! {
        seal::seal_pre_commit_phase2(phase1_output, cache_path, out_path)
    }
}

/// create tree-d file
pub fn create_tree_d(
    registered_proof: RegisteredSealProof,
    in_path: Option<PathBuf>,
    cache_path: PathBuf,
) -> Result<()> {
    safe_call! {
        proof::create_tree_d(
            registered_proof,
            in_path,
            cache_path,
        )
    }
}

pub fn tree_d_path_in_dir(dir: &PathBuf) -> PathBuf {
    StoreConfig::data_path(dir, &CacheKey::CommDTree.to_string())
}

pub fn snap_encode_into(
    registered_proof: RegisteredUpdateProof,
    new_replica_path: PathBuf,
    new_cache_path: PathBuf,
    sector_path: PathBuf,
    sector_cache_path: PathBuf,
    staged_data_path: PathBuf,
    piece_infos: &[PieceInfo],
) -> Result<EmptySectorUpdateEncoded> {
    safe_call! {
        empty_sector_update_encode_into(
            registered_proof,
            new_replica_path,
            new_cache_path,
            sector_path,
            sector_cache_path,
            staged_data_path,
            piece_infos,
        )
    }
}

pub fn snap_generate_partition_proofs(
    registered_proof: RegisteredUpdateProof,
    comm_r_old: Commitment,
    comm_r_new: Commitment,
    comm_d_new: Commitment,
    sector_path: PathBuf,
    sector_cache_path: PathBuf,
    replica_path: PathBuf,
    replica_cache_path: PathBuf,
) -> Result<Vec<PartitionProofBytes>> {
    safe_call! {
        generate_partition_proofs(
            registered_proof,
            comm_r_old,
            comm_r_new,
            comm_d_new,
            sector_path,
            sector_cache_path,
            replica_path,
            replica_cache_path,
        )
    }
}

pub fn snap_generate_sector_update_proof(
    registered_proof: RegisteredUpdateProof,
    vannilla_proofs: Vec<PartitionProofBytes>,
    comm_r_old: Commitment,
    comm_r_new: Commitment,
    comm_d_new: Commitment,
) -> Result<EmptySectorUpdateProof> {
    safe_call! {
        generate_empty_sector_update_proof_with_vanilla(
            registered_proof,
            vannilla_proofs,
            comm_r_old,
            comm_r_new,
            comm_d_new,
        )
    }
}
