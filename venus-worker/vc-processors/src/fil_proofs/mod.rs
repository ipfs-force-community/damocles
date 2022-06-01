//! This module provides types and apis re-exported from rust-fil-proofs
//!

use std::fs::OpenOptions;
use std::panic::catch_unwind;
use std::path::PathBuf;

use anyhow::{Context, Result};
use filecoin_proofs::{get_base_tree_leafs, get_base_tree_size, DefaultBinaryTree, DefaultPieceHasher, StoreConfig, BINARY_ARITY};
use filecoin_proofs_api::seal;
use memmap::{Mmap, MmapOptions};
use storage_proofs_core::{
    cache_key::CacheKey,
    merkle::{create_base_merkle_tree, BinaryMerkleTree},
    util::default_rows_to_discard,
};

// re-exported
pub use filecoin_proofs::{EmptySectorUpdateEncoded, EmptySectorUpdateProof};
pub use filecoin_proofs_api::{
    seal::{
        clear_cache, write_and_preprocess, Labels, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
        SealPreCommitPhase2Output,
    },
    update::{
        empty_sector_update_encode_into, generate_empty_sector_update_proof_with_vanilla, generate_partition_proofs,
        verify_empty_sector_update_proof, verify_partition_proofs,
    },
    Commitment, PaddedBytesAmount, PartitionProofBytes, PieceInfo, ProverId, RegisteredSealProof, RegisteredUpdateProof, SectorId, Ticket,
    UnpaddedBytesAmount,
};

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

pub fn snap_verify_sector_update_proof(
    registered_proof: RegisteredUpdateProof,
    proof: &[u8],
    comm_r_old: Commitment,
    comm_r_new: Commitment,
    comm_d_new: Commitment,
) -> Result<bool> {
    safe_call! {
        verify_empty_sector_update_proof(
            registered_proof,
            proof,
            comm_r_old,
            comm_r_new,
            comm_d_new,
        )
    }
}

pub fn create_tree_d(registered_proof: RegisteredSealProof, in_path: Option<PathBuf>, cache_path: PathBuf) -> Result<()> {
    safe_call! {
        create_tree_d_inner(
            registered_proof,
            in_path,
            cache_path,
        )
    }
}

enum Bytes {
    Mmap(Mmap),
    InMem(Vec<u8>),
}

impl AsRef<[u8]> for Bytes {
    #[inline]
    fn as_ref(&self) -> &[u8] {
        match self {
            Bytes::Mmap(m) => &m[..],
            Bytes::InMem(m) => &m[..],
        }
    }
}

fn create_tree_d_inner(registered_proof: RegisteredSealProof, in_path: Option<PathBuf>, cache_path: PathBuf) -> Result<()> {
    let sector_size = registered_proof.sector_size();
    let tree_size = get_base_tree_size::<DefaultBinaryTree>(sector_size)?;
    let tree_leafs = get_base_tree_leafs::<DefaultBinaryTree>(tree_size)?;

    let data = match in_path {
        Some(p) => {
            let f = OpenOptions::new()
                .read(true)
                .open(&p)
                .with_context(|| format!("open staged file {:?}", p))?;

            let mapped = unsafe { MmapOptions::new().map(&f).with_context(|| format!("mmap staged file: {:?}", p))? };

            Bytes::Mmap(mapped)
        }

        None => Bytes::InMem(vec![0; sector_size.0 as usize]),
    };

    let cfg = StoreConfig::new(
        &cache_path,
        CacheKey::CommDTree.to_string(),
        default_rows_to_discard(tree_leafs, BINARY_ARITY),
    );

    create_base_merkle_tree::<BinaryMerkleTree<DefaultPieceHasher>>(Some(cfg), tree_leafs, data.as_ref())?;

    Ok(())
}
