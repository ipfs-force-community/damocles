//! This module provides types and apis re-exported from rust-fil-proofs
//!

use std::collections::BTreeMap;
use std::fs::OpenOptions;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::path::PathBuf;

use anyhow::{Context, Result};
use filecoin_proofs::{get_base_tree_leafs, get_base_tree_size, DefaultBinaryTree, DefaultPieceHasher, StoreConfig, BINARY_ARITY};
use filecoin_proofs_api::post;
use filecoin_proofs_api::seal;
use forest_address::Address;
use memmap::{Mmap, MmapOptions};
use serde::{Deserialize, Serialize};
use storage_proofs_core::{
    cache_key::CacheKey,
    merkle::{create_base_merkle_tree, BinaryMerkleTree},
    util::default_rows_to_discard,
};

// re-exported
pub use filecoin_proofs_api::{
    seal::{
        clear_cache, write_and_preprocess, Labels, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
        SealPreCommitPhase2Output,
    },
    update::{
        empty_sector_update_encode_into, generate_empty_sector_update_proof_with_vanilla, generate_partition_proofs,
        verify_empty_sector_update_proof, verify_partition_proofs,
    },
    ChallengeSeed, Commitment, PaddedBytesAmount, PartitionProofBytes, PieceInfo, PrivateReplicaInfo, ProverId, RegisteredPoStProof,
    RegisteredSealProof, RegisteredUpdateProof, SectorId, Ticket, UnpaddedBytesAmount,
};

pub use storage_proofs_core::settings;

/// Identifier for Actors.
pub type ActorID = u64;

pub type SnarkProof = crate::b64serde::BytesVec;

macro_rules! safe_call {
    ($ex:expr) => {
        match catch_unwind(AssertUnwindSafe(move || $ex.map_err(|e| format!("{:?}", e)))) {
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

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SnapEncodeOutput {
    pub comm_r_new: Commitment,
    pub comm_r_last_new: Commitment,
    pub comm_d_new: Commitment,
}

pub fn snap_encode_into(
    registered_proof: RegisteredUpdateProof,
    new_replica_path: PathBuf,
    new_cache_path: PathBuf,
    sector_path: PathBuf,
    sector_cache_path: PathBuf,
    staged_data_path: PathBuf,
    piece_infos: &[PieceInfo],
) -> Result<SnapEncodeOutput> {
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
        .map(|out| SnapEncodeOutput {
            comm_r_new: out.comm_r_new,
            comm_r_last_new: out.comm_r_last_new,
            comm_d_new: out.comm_d_new,
        })
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

pub type SnapProveOutput = Vec<u8>;

pub fn snap_generate_sector_update_proof(
    registered_proof: RegisteredUpdateProof,
    vannilla_proofs: Vec<PartitionProofBytes>,
    comm_r_old: Commitment,
    comm_r_new: Commitment,
    comm_d_new: Commitment,
) -> Result<SnapProveOutput> {
    safe_call! {
        generate_empty_sector_update_proof_with_vanilla(
            registered_proof,
            vannilla_proofs,
            comm_r_old,
            comm_r_new,
            comm_d_new,
        ).map(|out| out.0)
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

pub fn tree_d_path_in_dir(dir: &PathBuf) -> PathBuf {
    StoreConfig::data_path(dir, &CacheKey::CommDTree.to_string())
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

pub fn create_tree_d(registered_proof: RegisteredSealProof, in_path: Option<PathBuf>, cache_path: PathBuf) -> Result<()> {
    safe_call! {
        create_tree_d_inner(
            registered_proof,
            in_path,
            cache_path,
        )
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

pub fn cached_filenames_for_sector(registered_proof: RegisteredSealProof) -> Vec<PathBuf> {
    use RegisteredSealProof::*;
    let mut trees = match registered_proof {
        StackedDrg2KiBV1 | StackedDrg8MiBV1 | StackedDrg512MiBV1 | StackedDrg2KiBV1_1 | StackedDrg8MiBV1_1 | StackedDrg512MiBV1_1 => {
            vec!["sc-02-data-tree-r-last.dat".into()]
        }

        StackedDrg32GiBV1 | StackedDrg32GiBV1_1 => (0..8).map(|idx| format!("sc-02-data-tree-r-last-{}.dat", idx).into()).collect(),

        StackedDrg64GiBV1 | StackedDrg64GiBV1_1 => (0..16).map(|idx| format!("sc-02-data-tree-r-last-{}.dat", idx).into()).collect(),
    };

    trees.push("p_aux".into());
    trees.push("t_aux".into());

    trees
}

pub fn to_prover_id(miner_id: ActorID) -> ProverId {
    let mut prover_id: ProverId = Default::default();
    let actor_addr_payload = Address::new_id(miner_id).payload_bytes();
    prover_id[..actor_addr_payload.len()].copy_from_slice(actor_addr_payload.as_ref());
    prover_id
}

pub fn generate_window_post(
    randomness: &ChallengeSeed,
    replicas: &BTreeMap<SectorId, PrivateReplicaInfo>,
    prover_id: ProverId,
) -> Result<Vec<(RegisteredPoStProof, SnarkProof)>> {
    safe_call! {
        post::generate_window_post(
            randomness,
            replicas,
            prover_id,
        )
    }
    .map(|proofs| proofs.into_iter().map(|(r, p)| (r, p.into())).collect())
}

pub fn generate_winning_post(
    randomness: &ChallengeSeed,
    replicas: &BTreeMap<SectorId, PrivateReplicaInfo>,
    prover_id: ProverId,
) -> Result<Vec<(RegisteredPoStProof, SnarkProof)>> {
    safe_call! {
        post::generate_winning_post(
            randomness,
            replicas,
            prover_id,
        )
    }
    .map(|proofs| proofs.into_iter().map(|(r, p)| (r, p.into())).collect())
}
