//! Built-in tasks.
//!

#[cfg(feature = "fil-proofs")]
use crate::fil_proofs::{
    ActorID, ChallengeSeed, Commitment, PieceInfo, ProverId, RegisteredPoStProof, RegisteredSealProof, RegisteredUpdateProof,
    SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output, SealPreCommitPhase2Output, SectorId, SnapEncodeOutput,
    SnapProveOutput, SnarkProof, Ticket,
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

#[cfg(feature = "builtin-add-pieces")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum PieceFile {
    Url(String),
    Local(std::path::PathBuf),
    Pledge,
}

#[cfg(feature = "builtin-add-pieces")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct Piece {
    pub piece_file: PieceFile,
    pub payload_size: u64,
    pub piece_size: filecoin_proofs::UnpaddedBytesAmount,
}

/// Task of add_piece
#[cfg(feature = "builtin-add-pieces")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct AddPieces {
    pub seal_proof_type: RegisteredSealProof,
    pub pieces: Vec<Piece>,
    pub staged_filepath: std::path::PathBuf,
}

#[cfg(feature = "builtin-add-pieces")]
impl vc_processors::Task for AddPieces {
    const STAGE: &'static str = STAGE_NAME_ADD_PIECES;

    type Output = Vec<PieceInfo>;
}

/// Task of tree_d
#[cfg(feature = "builtin-tree-d")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct TreeD {
    pub registered_proof: RegisteredSealProof,
    pub staged_file: std::path::PathBuf,
    pub cache_dir: std::path::PathBuf,
}

#[cfg(feature = "builtin-tree-d")]
impl vc_processors::Task for TreeD {
    const STAGE: &'static str = STAGE_NAME_TREED;
    type Output = bool;
}

/// Task of pre-commit phase1
#[cfg(feature = "builtin-pc1")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct PC1 {
    pub registered_proof: RegisteredSealProof,
    pub cache_path: std::path::PathBuf,
    pub in_path: std::path::PathBuf,
    pub out_path: std::path::PathBuf,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub ticket: Ticket,
    pub piece_infos: Vec<PieceInfo>,
}

#[cfg(feature = "builtin-pc1")]
impl vc_processors::Task for PC1 {
    const STAGE: &'static str = STAGE_NAME_PC1;
    type Output = SealPreCommitPhase1Output;
}

/// Task of pre-commit phase2
#[cfg(feature = "builtin-pc2")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct PC2 {
    pub pc1out: SealPreCommitPhase1Output,
    pub cache_dir: std::path::PathBuf,
    pub sealed_file: std::path::PathBuf,
}

#[cfg(feature = "builtin-pc2")]
impl vc_processors::Task for PC2 {
    const STAGE: &'static str = STAGE_NAME_PC2;
    type Output = SealPreCommitPhase2Output;
}

/// Task of commit phase2
#[cfg(feature = "builtin-c2")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct C2 {
    pub c1out: SealCommitPhase1Output,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub miner_id: ActorID,
}

#[cfg(feature = "builtin-c2")]
impl vc_processors::Task for C2 {
    const STAGE: &'static str = STAGE_NAME_C2;
    type Output = SealCommitPhase2Output;
}

/// Task of snap encode
#[cfg(feature = "builtin-snap-encode")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct SnapEncode {
    pub registered_proof: RegisteredUpdateProof,
    pub new_replica_path: std::path::PathBuf,
    pub new_cache_path: std::path::PathBuf,
    pub sector_path: std::path::PathBuf,
    pub sector_cache_path: std::path::PathBuf,
    pub staged_data_path: std::path::PathBuf,
    pub piece_infos: Vec<PieceInfo>,
}

#[cfg(feature = "builtin-snap-encode")]
impl vc_processors::Task for SnapEncode {
    const STAGE: &'static str = STAGE_NAME_SNAP_ENCODE;
    type Output = SnapEncodeOutput;
}

/// Task of snap prove
#[cfg(feature = "builtin-snap-prove")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct SnapProve {
    pub registered_proof: RegisteredUpdateProof,
    pub vannilla_proofs: Vec<Vec<u8>>,
    pub comm_r_old: Commitment,
    pub comm_r_new: Commitment,
    pub comm_d_new: Commitment,
}

#[cfg(feature = "builtin-snap-prove")]
impl vc_processors::Task for SnapProve {
    const STAGE: &'static str = STAGE_NAME_SNAP_PROVE;
    type Output = SnapProveOutput;
}

#[cfg(feature = "builtin-transfer")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct TransferStoreInfo {
    pub name: String,
    pub meta: Option<std::collections::HashMap<String, String>>,
}

#[cfg(feature = "builtin-transfer")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct TransferItem {
    pub store_name: Option<String>,
    pub uri: std::path::PathBuf,
}

#[cfg(feature = "builtin-transfer")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct TransferOption {
    pub is_dir: bool,
    pub allow_link: bool,
}

#[cfg(feature = "builtin-transfer")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct TransferRoute {
    pub src: TransferItem,
    pub dest: TransferItem,
    pub opt: Option<TransferOption>,
}

#[cfg(feature = "builtin-transfer")]
/// Task of transfer
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct Transfer {
    /// store infos used in transfer items
    pub stores: std::collections::HashMap<String, TransferStoreInfo>,

    pub routes: Vec<TransferRoute>,
}

#[cfg(feature = "builtin-transfer")]
impl vc_processors::Task for Transfer {
    const STAGE: &'static str = STAGE_NAME_TRANSFER;

    type Output = bool;
}

#[cfg(any(feature = "builtin-window-post", feature = "builtin-winning-post"))]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct PoStReplicaInfo {
    pub sector_id: SectorId,
    pub comm_r: Commitment,
    pub cache_dir: std::path::PathBuf,
    pub sealed_file: std::path::PathBuf,
}

#[cfg(feature = "builtin-window-post")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct WindowPoStOutput {
    pub proofs: Vec<SnarkProof>,
    pub faults: Vec<u64>,
}

#[cfg(feature = "builtin-window-post")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct WindowPoSt {
    pub miner_id: ActorID,
    pub proof_type: RegisteredPoStProof,
    pub replicas: Vec<PoStReplicaInfo>,
    pub seed: ChallengeSeed,
}

#[cfg(feature = "builtin-window-post")]
impl vc_processors::Task for WindowPoSt {
    const STAGE: &'static str = STAGE_NAME_WINDOW_POST;

    type Output = WindowPoStOutput;
}

#[cfg(feature = "builtin-winning-post")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct WinningPoStOutput {
    pub proofs: Vec<SnarkProof>,
}

#[cfg(feature = "builtin-winning-post")]
#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct WinningPoSt {
    pub miner_id: ActorID,
    pub proof_type: RegisteredPoStProof,
    pub replicas: Vec<PoStReplicaInfo>,
    pub seed: ChallengeSeed,
}

#[cfg(feature = "builtin-winning-post")]
impl vc_processors::Task for WinningPoSt {
    const STAGE: &'static str = STAGE_NAME_WINNING_POST;

    type Output = WinningPoStOutput;
}
