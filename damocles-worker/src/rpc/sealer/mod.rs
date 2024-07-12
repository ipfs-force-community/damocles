use std::collections::HashMap;
use std::fmt::Display;
use std::path::PathBuf;

use super::super::types::SealProof;
use crate::sealing::processor::ChallengeSeed;
use fil_clock::ChainEpoch;
use fil_types::{ActorID, PaddedPieceSize, SectorNumber};
use forest_cid::json::CidJson;
use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};
use serde_repr::{Deserialize_repr, Serialize_repr};
use vc_processors::b64serde::{BytesArray32, BytesVec};
use vc_processors::builtin::tasks::WindowPoStOutput;
use vc_processors::fil_proofs::{
    Commitment, PaddedBytesAmount, RegisteredPoStProof, SectorId,
};

/// type alias for BytesArray32
pub type Randomness = BytesArray32;

pub type B64Vec = BytesVec;

/// type alias for u64
pub type DealID = u64;

/// contains miner actor id & sector number
#[derive(Clone, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorID {
    /// miner actor id
    pub miner: ActorID,

    /// sector number
    pub number: SectorNumber,
}

impl Display for SectorID {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "s-t0{}-{}", self.miner, self.number)
    }
}

impl std::fmt::Debug for SectorID {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self)
    }
}

/// rules for allocating sector bases
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct AllocateSectorSpec {
    /// specified miner actor ids
    pub allowed_miners: Option<Vec<ActorID>>,

    /// specified seal proof types
    pub allowed_proof_types: Option<Vec<SealProof>>,
}

/// basic infos for a allocated sector
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct AllocatedSector {
    /// allocated sector id
    #[serde(rename = "ID")]
    pub id: SectorID,

    /// allocated seal proof type
    pub proof_type: SealProof,
}

/// deal piece info
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct DealInfo {
    /// on-chain deal id
    #[serde(rename = "ID")]
    pub id: DealID,

    pub payload_size: u64,

    /// piece data info
    pub piece: PieceInfo,
}

/// sector piece info
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorPiece {
    pub piece: PieceInfoV2,

    /// piece data info
    pub deal_info: DealInfoV2,
}

/// deal piece info v2
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct DealInfoV2 {
    #[serde(rename = "DealID")]
    pub deal_id: DealID,
    #[serde(rename = "AllocationID")]
    pub allocation_id: u64,
    pub payload_size: u64,
    pub is_builtin_market: bool,
}

/// Piece information for part or a whole file.
#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct PieceInfo {
    /// Size in nodes. For BLS12-381 (capacity 254 bits), must be >= 16. (16 * 8 = 128).
    pub size: PaddedPieceSize,
    /// Offset within the original file to the first byte of this piece.
    #[serde(default = "default_padded_byte_amount")]
    pub offset: PaddedBytesAmount,
    /// Content identifier for piece.
    pub cid: CidJson,
}

/// Piece information for part or a whole file.
#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct PieceInfoV2 {
    /// Size in nodes. For BLS12-381 (capacity 254 bits), must be >= 16. (16 * 8 = 128).
    pub size: PaddedPieceSize,
    #[serde(rename = "PieceCID")]
    pub piece_cid: CidJson,
}

fn default_padded_byte_amount() -> PaddedBytesAmount {
    PaddedBytesAmount(0)
}

/// types alias for deal piece info list
pub type Deals = Vec<DealInfo>;
pub type DealsV2 = Vec<SectorPiece>;

/// rules for acquiring deal pieces within specified sector
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct AcquireDealsSpec {
    /// max deal count
    pub max_deals: Option<usize>,

    pub min_used_space: Option<usize>,
}

/// assigned ticket
#[derive(Debug, Clone, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "PascalCase")]
pub struct Ticket {
    /// raw ticket data
    pub ticket: Randomness,

    /// chain epoch from which ticket is fetched
    pub epoch: ChainEpoch,
}

/// results for pre_commit & proof submission
#[derive(Debug, Deserialize_repr, Serialize_repr)]
#[repr(u64)]
pub enum SubmitResult {
    /// submission is accepted
    Accepted = 1,

    /// submission duplicates w/o content change
    DuplicateSubmit = 2,

    /// submission duplicates w/ content change
    MismatchedSubmission = 3,

    /// submission is rejected for some reason
    Rejected = 4,

    FilesMissed = 5,
}

/// state for submitted pre_commit or proof
#[derive(Debug, Deserialize_repr, Serialize_repr)]
#[repr(u64)]
pub enum OnChainState {
    /// waiting to be sent or aggregated
    Pending = 1,

    /// assigned to a msg
    Packed = 2,

    /// msg landed on chain
    Landed = 3,

    /// sector not found
    NotFound = 4,

    /// on chain msg exec failed
    Failed = 5,

    /// permanent failed
    PermFailed = 6,

    /// the sector is not going to get on-chain
    ShouldAbort = 7,
}

/// required infos for pre commint
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct PreCommitOnChainInfo {
    /// commitment replicate
    pub comm_r: [u8; 32],

    /// commitment data
    pub comm_d: [u8; 32],

    /// assigned ticket
    pub ticket: Ticket,
}

/// required infos for proof
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct ProofOnChainInfo {
    /// proof bytes
    pub proof: B64Vec,
}

/// response for the submit_pre_commit request
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SubmitPreCommitResp {
    /// submit result
    pub res: SubmitResult,

    /// description
    pub desc: Option<String>,
}

/// response for the poll_pre_commit_state request
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct PollPreCommitStateResp {
    /// on chain state
    pub state: OnChainState,

    /// description
    pub desc: Option<String>,
}

/// assigned seed
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct Seed {
    /// raw seed data
    pub seed: Randomness,

    /// chain epoch from which seed is fetched
    pub epoch: ChainEpoch,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct WaitSeedResp {
    pub should_wait: bool,
    pub delay: u64,
    pub seed: Option<Seed>,
}

/// response for the submit_proof request
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SubmitProofResp {
    /// submit result
    pub res: SubmitResult,

    /// description
    pub desc: Option<String>,
}

/// response for the poll_proof_state request
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct PollProofStateResp {
    /// on chain state
    pub state: OnChainState,

    /// description
    pub desc: Option<String>,
}

#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct WorkerIdentifier {
    pub instance: String,
    pub location: PathBuf,
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "PascalCase")]
pub struct ReportStateReq {
    pub worker: WorkerIdentifier,
    pub state_change: SectorStateChange,
    pub failure: Option<SectorFailure>,
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "PascalCase")]
pub struct SectorState {
    pub id: SectorID,
    pub finalized: bool,
    pub abort_reason: Option<String>,
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "PascalCase")]
pub struct SectorStateChange {
    pub prev: String,
    pub next: String,
    pub event: String,
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "PascalCase")]
pub struct SectorFailure {
    pub level: String,
    pub desc: String,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct AllocateSnapUpSpec {
    pub sector: AllocateSectorSpec,
    pub deals: AcquireDealsSpec,
}

#[derive(Default, Debug, Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorPublicInfo {
    pub comm_r: [u8; 32],
}

#[derive(Default, Debug, Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorPrivateInfo {
    // for now, snap up allocator only allow non-splited sectors
    pub access_instance: String,
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "PascalCase")]
pub struct AllocatedSnapUpSector {
    pub sector: AllocatedSector,
    pub pieces: DealsV2,
    pub public: SectorPublicInfo,
    pub private: SectorPrivateInfo,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SnapUpOnChainInfo {
    pub comm_r: [u8; 32],
    pub comm_d: [u8; 32],
    pub access_instance: String,
    pub pieces: Vec<CidJson>,
    pub proof: B64Vec,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SubmitSnapUpProofResp {
    pub res: SubmitResult,
    pub desc: Option<String>,
}

#[derive(Deserialize, Serialize, Default)]
#[serde(rename_all = "PascalCase")]
pub struct WorkerInfoSummary {
    pub threads: usize,
    pub empty: usize,
    pub paused: usize,
    pub running: usize,
    pub waiting: usize,
    pub errors: usize,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct WorkerInfo {
    pub name: String,
    pub dest: String,
    pub version: String,
    pub summary: WorkerInfoSummary,
}

#[derive(Deserialize, Serialize, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct StoreBasicInfo {
    pub name: String,
    pub path: String,
    pub meta: Option<HashMap<String, String>>,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorRebuildInfo {
    pub sector: AllocatedSector,
    pub ticket: Ticket,
    pub pieces: Option<Deals>,
    pub pieces_v2: Option<DealsV2>,

    #[serde(rename = "IsSnapUp")]
    pub is_snapup: bool,
    pub upgrade_public: Option<SectorPublicInfo>,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorUnsealInfo {
    pub sector: AllocatedSector,
    pub piece_cid: CidJson,
    pub comm_d: [u8; 32],
    pub ticket: Ticket,

    pub offset: u64,
    pub size: u64,

    pub private_info: SectorPrivateInfo,
}

#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct SectorAccessStores {
    pub sealed_file: String, // name for storage instance
    pub cache_dir: String,
}

/// rules for allocating sector bases
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct AllocatePoStSpec {
    /// specified miner actor ids
    pub allowed_miners: Option<Vec<ActorID>>,

    /// specified seal proof types
    pub allowed_proof_types: Option<Vec<RegisteredPoStProof>>,
}

#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct WdPoStSectorInfo {
    pub sector_id: SectorId,
    pub comm_r: Commitment,
    pub upgrade: bool, // is upgrade sector
    pub accesses: SectorAccessStores,
}

#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct WdPoStInput {
    pub sectors: Vec<WdPoStSectorInfo>,
    pub miner_id: ActorID,
    pub proof_type: RegisteredPoStProof,
    pub seed: ChallengeSeed,
}

#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct AllocatedWdPoStJob {
    pub id: String,
    pub input: WdPoStInput,
}

/// defines the SealerRpc service
#[rpc]
pub trait Sealer {
    /// api definition
    #[rpc(name = "Venus.AllocateSector")]
    fn allocate_sector(
        &self,
        spec: AllocateSectorSpec,
        niporep: bool,
    ) -> Result<Option<AllocatedSector>>;

    /// api definition
    #[rpc(name = "Venus.AllocateSectorsBatch")]
    fn allocate_sectors_batch(
        &self,
        spec: AllocateSectorSpec,
        niporep: bool,
        count: u32,
    ) -> Result<Vec<AllocatedSector>>;

    /// api definition
    #[rpc(name = "Venus.AcquireDeals")]
    fn acquire_deals(
        &self,
        id: SectorID,
        spec: AcquireDealsSpec,
    ) -> Result<Option<DealsV2>>;

    /// api definition
    #[rpc(name = "Venus.AssignTicket")]
    fn assign_ticket(&self, id: SectorID) -> Result<Ticket>;

    /// api definition
    #[rpc(name = "Venus.SubmitPreCommit")]
    fn submit_pre_commit(
        &self,
        sector: AllocatedSector,
        info: PreCommitOnChainInfo,
        reset: bool,
    ) -> Result<SubmitPreCommitResp>;

    /// api definition
    #[rpc(name = "Venus.PollPreCommitState")]
    fn poll_pre_commit_state(
        &self,
        id: SectorID,
    ) -> Result<PollPreCommitStateResp>;

    ///// api definition
    // #[rpc(name = "Venus.SubmitPersisted")]
    // fn submit_persisted(&self, id: SectorID, instance: String) -> Result<bool>;

    /// api definition
    #[rpc(name = "Venus.SubmitPersistedEx")]
    fn submit_persisted_ex(
        &self,
        id: SectorID,
        instance: String,
        is_upgrade: bool,
    ) -> Result<bool>;

    /// api definition
    #[rpc(name = "Venus.WaitSeed")]
    fn wait_seed(
        &self,
        id: SectorID,
        proof_type: SealProof,
    ) -> Result<WaitSeedResp>;

    /// api definition
    #[rpc(name = "Venus.SubmitProof")]
    fn submit_proof(
        &self,
        id: SectorID,
        proof: ProofOnChainInfo,
        reset: bool,
    ) -> Result<SubmitProofResp>;

    /// api definition
    #[rpc(name = "Venus.PollProofState")]
    fn poll_proof_state(&self, id: SectorID) -> Result<PollProofStateResp>;

    /// api definition
    #[rpc(name = "Venus.ReportState")]
    fn report_state(
        &self,
        id: SectorID,
        state: ReportStateReq,
    ) -> Result<SectorState>;

    /// api definition
    #[rpc(name = "Venus.ReportFinalized")]
    fn report_finalized(&self, id: SectorID) -> Result<()>;

    /// api definition
    #[rpc(name = "Venus.ReportAborted")]
    fn report_aborted(&self, id: SectorID, reason: String) -> Result<()>;

    // snap up
    /// api definition
    #[rpc(name = "Venus.AllocateSanpUpSector")]
    fn allocate_snapup_sector(
        &self,
        spec: AllocateSnapUpSpec,
    ) -> Result<Option<AllocatedSnapUpSector>>;

    /// api definition
    #[rpc(name = "Venus.SubmitSnapUpProof")]
    fn submit_snapup_proof(
        &self,
        id: SectorID,
        snapup_info: SnapUpOnChainInfo,
    ) -> Result<SubmitSnapUpProofResp>;

    #[rpc(name = "Venus.WorkerPing")]
    fn worker_ping(&self, winfo: WorkerInfo) -> Result<()>;

    #[rpc(name = "Venus.StoreReserveSpace")]
    fn store_reserve_space(
        &self,
        id: SectorID,
        size: u64,
        candidates: Vec<String>,
    ) -> Result<Option<StoreBasicInfo>>;

    #[rpc(name = "Venus.StoreBasicInfo")]
    fn store_basic_info(
        &self,
        instance_name: String,
    ) -> Result<Option<StoreBasicInfo>>;

    // rebuild
    #[rpc(name = "Venus.AllocateRebuildSector")]
    fn allocate_rebuild_sector(
        &self,
        spec: AllocateSectorSpec,
    ) -> Result<Option<SectorRebuildInfo>>;

    // unseal
    #[rpc(name = "Venus.AllocateUnsealSector")]
    fn allocate_unseal_sector(
        &self,
        spec: AllocateSectorSpec,
    ) -> Result<Option<SectorUnsealInfo>>;

    #[rpc(name = "Venus.AchieveUnsealSector")]
    fn achieve_unseal_sector(
        &self,
        id: SectorID,
        piece_cid: CidJson,
        error: String,
    ) -> Result<()>;

    #[rpc(name = "Venus.AcquireUnsealDest")]
    fn acquire_unseal_dest(
        &self,
        id: SectorID,
        piece_cid: CidJson,
    ) -> Result<Vec<String>>;

    #[rpc(name = "Venus.WdPoStAllocateJobs")]
    fn allocate_wdpost_job(
        &self,
        spec: AllocatePoStSpec,
        num: u32,
        worker_name: String,
    ) -> Result<Vec<AllocatedWdPoStJob>>;

    #[rpc(name = "Venus.WdPoStHeartbeatJobs")]
    fn wdpost_heartbeat(
        &self,
        job_ids: Vec<String>,
        worker_name: String,
    ) -> Result<()>;

    #[rpc(name = "Venus.WdPoStFinishJob")]
    fn wdpost_finish(
        &self,
        job_id: String,
        output: Option<WindowPoStOutput>,
        error_reason: String,
    ) -> Result<()>;
}
