use std::convert::TryFrom;
use std::path::PathBuf;
use std::result::Result as StdResult;

use anyhow::{anyhow, Error};
use base64::STANDARD;
use base64_serde::base64_serde_type;
use fil_clock::ChainEpoch;
use fil_types::{ActorID, PaddedPieceSize, SectorNumber};
use forest_cid::Cid;
use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};
use serde_repr::{Deserialize_repr, Serialize_repr};

use super::super::types::SealProof;

pub mod mock;

base64_serde_type! {B64SerDe, STANDARD}

/// randomness with base64 ser & de
#[derive(Copy, Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(into = "B64Vec")]
#[serde(try_from = "B64Vec")]
pub struct BytesArray32(pub [u8; 32]);

/// type alias for BytesArray32
pub type Randomness = BytesArray32;

impl From<[u8; 32]> for BytesArray32 {
    fn from(val: [u8; 32]) -> Self {
        BytesArray32(val)
    }
}

impl TryFrom<B64Vec> for BytesArray32 {
    type Error = Error;

    fn try_from(v: B64Vec) -> StdResult<Self, Self::Error> {
        if v.0.len() != 32 {
            return Err(anyhow!("expected 32 bytes, got {}", v.0.len()));
        }

        let mut a = [0u8; 32];
        a.copy_from_slice(&v.0[..]);

        Ok(BytesArray32(a))
    }
}

impl From<BytesArray32> for B64Vec {
    fn from(r: BytesArray32) -> Self {
        let mut v = vec![0; 32];
        v.copy_from_slice(&r.0[..]);
        B64Vec(v)
    }
}

#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(transparent)]
/// bytes with base64 ser & de
pub struct B64Vec(#[serde(with = "B64SerDe")] pub Vec<u8>);

impl From<Vec<u8>> for B64Vec {
    fn from(val: Vec<u8>) -> B64Vec {
        B64Vec(val)
    }
}

/// type alias for u64
pub type DealID = u64;

/// contains miner actor id & sector number
#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct SectorID {
    /// miner actor id
    pub miner: ActorID,

    /// sector number
    pub number: SectorNumber,
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

    /// piece data info
    pub piece: PieceInfo,
}

/// Piece information for part or a whole file.
#[derive(Debug, Serialize, Deserialize, PartialEq, Clone)]
#[serde(rename_all = "PascalCase")]
pub struct PieceInfo {
    /// Size in nodes. For BLS12-381 (capacity 254 bits), must be >= 16. (16 * 8 = 128).
    pub size: PaddedPieceSize,
    /// Content identifier for piece.
    pub cid: Cid,
}

/// types alias for deal piece info list
pub type Deals = Vec<DealInfo>;

/// rules for acquiring deal pieces within specified sector
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
pub struct AcquireDealsSpec {
    /// max deal count
    pub max_deals: Option<usize>,
}

/// assigned ticket
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
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

    /// included deal ids
    pub deals: Vec<DealID>,
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

#[derive(Deserialize, Serialize, Debug)]
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
    pub failure: Option<Failure>,
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
pub struct Failure {
    pub level: String,
    pub desc: String,
}

/// defines the SealerRpc service
#[rpc]
pub trait Sealer {
    /// api definition
    #[rpc(name = "Venus.AllocateSector")]
    fn allocate_sector(&self, spec: AllocateSectorSpec) -> Result<Option<AllocatedSector>>;

    /// api definition
    #[rpc(name = "Venus.AcquireDeals")]
    fn acquire_deals(&self, id: SectorID, spec: AcquireDealsSpec) -> Result<Option<Deals>>;

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
    fn poll_pre_commit_state(&self, id: SectorID) -> Result<PollPreCommitStateResp>;

    /// api definition
    #[rpc(name = "Venus.WaitSeed")]
    fn wait_seed(&self, id: SectorID) -> Result<WaitSeedResp>;

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
    fn report_state(&self, id: SectorID, req: ReportStateReq) -> Result<()>;

    /// api definition
    #[rpc(name = "Venus.ReportFinalized")]
    fn report_finalized(&self, id: SectorID) -> Result<()>;
}
