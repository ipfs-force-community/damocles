//! Built-in tasks.
//!

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use filecoin_proofs::{UnpaddedByteIndex, UnpaddedBytesAmount};
use serde::{Deserialize, Serialize};

use crate::core::Task;
use crate::fil_proofs::{
    ActorID, ChallengeSeed, Commitment, PieceInfo, ProverId,
    RegisteredPoStProof, RegisteredSealProof, RegisteredUpdateProof,
    SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, SectorId, SnapEncodeOutput, SnapProveOutput,
    SnarkProof, Ticket,
};

/// name str for add_pieces
pub const STAGE_NAME_ADD_PIECES: &str = "add_pieces";

/// name str for tree_d
pub const STAGE_NAME_TREED: &str = "tree_d";

/// name str for pc1
pub const STAGE_NAME_PC1: &str = "pc1";

/// name str for pc2
pub const STAGE_NAME_PC2: &str = "pc2";

/// name str for c1
pub const STAGE_NAME_C1: &str = "c1";

/// name str for c2
pub const STAGE_NAME_C2: &str = "c2";

/// name str for snap encode
pub const STAGE_NAME_SNAP_ENCODE: &str = "snap_encode";

/// name str for snap prove
pub const STAGE_NAME_SNAP_PROVE: &str = "snap_prove";

/// name str for data transfer
pub const STAGE_NAME_TRANSFER: &str = "transfer";

/// name str for window post
pub const STAGE_NAME_WINDOW_POST: &str = "window_post";

/// name str for window post
pub const STAGE_NAME_WINNING_POST: &str = "winning_post";

/// name str for unseal
pub const STAGE_NAME_UNSEAL: &str = "unseal";

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum PieceFile {
    Url(String),
    Local(PathBuf),
    Pledge,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Piece {
    pub piece_file: PieceFile,
    pub payload_size: u64,
    pub piece_size: UnpaddedBytesAmount,
}

/// Task of add_piece
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct AddPieces {
    pub seal_proof_type: RegisteredSealProof,
    pub pieces: Vec<Piece>,
    pub staged_filepath: PathBuf,
}

impl Task for AddPieces {
    const STAGE: &'static str = STAGE_NAME_ADD_PIECES;

    type Output = Vec<PieceInfo>;
}

/// Task of tree_d
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TreeD {
    pub registered_proof: RegisteredSealProof,
    pub staged_file: PathBuf,
    pub cache_dir: PathBuf,
}

impl Task for TreeD {
    const STAGE: &'static str = STAGE_NAME_TREED;
    type Output = bool;
}

/// Task of pre-commit phase1
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PC1 {
    pub registered_proof: RegisteredSealProof,
    pub cache_path: PathBuf,
    pub in_path: PathBuf,
    pub out_path: PathBuf,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub ticket: Ticket,
    pub piece_infos: Vec<PieceInfo>,
}

impl Task for PC1 {
    const STAGE: &'static str = STAGE_NAME_PC1;
    type Output = SealPreCommitPhase1Output;
}

/// Task of pre-commit phase2
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PC2 {
    pub pc1out: SealPreCommitPhase1Output,
    pub cache_dir: PathBuf,
    pub sealed_file: PathBuf,
}

impl Task for PC2 {
    const STAGE: &'static str = STAGE_NAME_PC2;
    type Output = SealPreCommitPhase2Output;
}

/// Task of commit phase2
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct C2 {
    pub c1out: SealCommitPhase1Output,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub miner_id: ActorID,
}

impl Task for C2 {
    const STAGE: &'static str = STAGE_NAME_C2;
    type Output = SealCommitPhase2Output;
}

/// Task of snap encode
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SnapEncode {
    pub registered_proof: RegisteredUpdateProof,

    pub new_replica_path: PathBuf,

    pub new_cache_path: PathBuf,

    pub sector_path: PathBuf,

    pub sector_cache_path: PathBuf,

    pub staged_data_path: PathBuf,

    pub piece_infos: Vec<PieceInfo>,
}

impl Task for SnapEncode {
    const STAGE: &'static str = STAGE_NAME_SNAP_ENCODE;
    type Output = SnapEncodeOutput;
}

/// Task of snap prove
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SnapProve {
    pub registered_proof: RegisteredUpdateProof,

    pub vannilla_proofs: Vec<Vec<u8>>,

    pub comm_r_old: Commitment,

    pub comm_r_new: Commitment,

    pub comm_d_new: Commitment,
}

impl Task for SnapProve {
    const STAGE: &'static str = STAGE_NAME_SNAP_PROVE;
    type Output = SnapProveOutput;
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferStoreInfo {
    pub name: String,
    pub meta: Option<HashMap<String, String>>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub enum TransferItem {
    Store { store: String, path: PathBuf },
    Local(PathBuf),
}

impl TransferItem {
    pub fn path(&self) -> &Path {
        match self {
            TransferItem::Store { path, .. } => path,
            TransferItem::Local(path) => path,
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferOption {
    pub is_dir: bool,
    pub allow_link: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferRoute {
    pub src: TransferItem,
    pub dest: TransferItem,
    pub opt: Option<TransferOption>,
}

/// Task of transfer
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Transfer {
    /// store infos used in transfer items
    pub stores: HashMap<String, TransferStoreInfo>,

    pub routes: Vec<TransferRoute>,
}

impl Task for Transfer {
    const STAGE: &'static str = STAGE_NAME_TRANSFER;

    type Output = bool;
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PoStReplicaInfo {
    pub sector_id: SectorId,
    pub comm_r: Commitment,
    pub cache_dir: PathBuf,
    pub sealed_file: PathBuf,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WindowPoStOutput {
    pub proofs: Vec<SnarkProof>,
    pub faults: Vec<u64>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WindowPoSt {
    pub miner_id: ActorID,
    pub proof_type: RegisteredPoStProof,
    pub replicas: Vec<PoStReplicaInfo>,
    pub seed: ChallengeSeed,
}

impl Task for WindowPoSt {
    const STAGE: &'static str = STAGE_NAME_WINDOW_POST;

    type Output = WindowPoStOutput;
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WinningPoStOutput {
    pub proofs: Vec<SnarkProof>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WinningPoSt {
    pub miner_id: ActorID,
    pub proof_type: RegisteredPoStProof,
    pub replicas: Vec<PoStReplicaInfo>,
    pub seed: ChallengeSeed,
}

impl Task for WinningPoSt {
    const STAGE: &'static str = STAGE_NAME_WINNING_POST;

    type Output = WinningPoStOutput;
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Unseal {
    pub registered_proof: RegisteredSealProof,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub comm_d: Commitment,
    pub ticket: Ticket,
    pub cache_dir: PathBuf,
    pub sealed_file: PathBuf,
    pub unsealed_output: PathBuf,

    pub offset: UnpaddedByteIndex,
    pub num_bytes: UnpaddedBytesAmount,
}

impl Task for Unseal {
    const STAGE: &'static str = STAGE_NAME_UNSEAL;

    type Output = UnpaddedBytesAmount;
}
