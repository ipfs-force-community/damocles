use std::convert::TryFrom;
use std::result::Result as StdResult;

use anyhow::{anyhow, Error};
use base64::STANDARD;
use base64_serde::base64_serde_type;
use fil_clock::ChainEpoch;
use fil_types::{ActorID, PieceInfo, SectorNumber};
use filecoin_proofs_api::{Commitment, RegisteredSealProof};
use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

base64_serde_type! {B64SerDe, STANDARD}

/// randomness with base64 ser & de
#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(into = "B64Vec")]
#[serde(try_from = "B64Vec")]
pub struct Randomness(pub [u8; 32]);

impl TryFrom<B64Vec> for Randomness {
    type Error = Error;

    fn try_from(v: B64Vec) -> StdResult<Self, Self::Error> {
        if v.0.len() != 32 {
            return Err(anyhow!("expected 32 bytes, got {}", v.0.len()));
        }

        let mut a = [0u8; 32];
        a.copy_from_slice(&v.0[..]);

        Ok(Randomness(a))
    }
}

impl From<Randomness> for B64Vec {
    fn from(r: Randomness) -> Self {
        let mut v = vec![0; 32];
        v.copy_from_slice(&r.0[..]);
        B64Vec(v)
    }
}

#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(transparent)]
/// bytes with base64 ser & de
pub struct B64Vec(#[serde(with = "B64SerDe")] pub Vec<u8>);

/// provides mock impl for the SealerRpc
pub mod mock;

/// type alias for u64
pub type DealID = u64;

/// contains miner actor id & sector number
#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
pub struct SectorID {
    /// miner actor id
    pub miner: ActorID,

    /// sector number
    pub number: SectorNumber,
}

/// rules for allocating sector bases
#[derive(Serialize, Deserialize)]
pub struct AllocateSectorSpec {
    /// specified miner actor ids
    pub allowed_miners: Option<Vec<ActorID>>,

    /// specified seal proof types
    pub allowed_proof_types: Option<Vec<RegisteredSealProof>>,
}

/// basic infos for a allocated sector
#[derive(Clone, Serialize, Deserialize)]
pub struct AllocatedSector {
    /// allocated sector id
    pub id: SectorID,

    /// allocated seal proof type
    pub proof_type: RegisteredSealProof,
}

/// deal piece info
#[derive(Clone, Serialize, Deserialize)]
pub struct DealInfo {
    /// on-chain deal id
    pub id: DealID,

    /// piece data info
    pub piece: PieceInfo,
}

/// types alias for deal piece info list
pub type Deals = Vec<DealInfo>;

/// rules for acquiring deal pieces within specified sector
#[derive(Serialize, Deserialize)]
pub struct AcquireDealsSpec {
    /// max deal count
    pub max_deals: Option<usize>,
}

/// assigned ticket
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct Ticket {
    /// raw ticket data
    pub ticket: Randomness,

    /// chain epoch from which ticket is fetched
    pub epoch: ChainEpoch,
}

/// results for pre_commit & proof submission
#[derive(Debug, Deserialize, Serialize)]
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
#[derive(Debug, Deserialize, Serialize)]
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
}

/// required infos for pre commint
#[derive(Deserialize, Serialize)]
pub struct PreCommitOnChainInfo {
    /// commitment replicate
    pub comm_r: Commitment,

    /// assigned ticket
    pub ticket: Ticket,

    /// included deal ids
    pub deals: Vec<DealID>,
}

/// required infos for proof
#[derive(Deserialize, Serialize)]
pub struct ProofOnChainInfo {
    /// proof bytes
    pub proof: Vec<u8>,
}

/// response for the submit_pre_commit request
#[derive(Deserialize, Serialize)]
pub struct SubmitPreCommitResp {
    /// submit result
    pub res: SubmitResult,

    /// description
    pub desc: Option<String>,
}

/// response for the poll_pre_commit_state request
#[derive(Deserialize, Serialize)]
pub struct PollPreCommitStateResp {
    /// on chain state
    pub state: OnChainState,

    /// description
    pub desc: Option<String>,
}

/// assigned seed
#[derive(Clone, Default, Deserialize, Serialize)]
pub struct Seed {
    /// raw seed data
    pub seed: Randomness,

    /// chain epoch from which seed is fetched
    pub epoch: ChainEpoch,
}

/// response for the submit_proof request
#[derive(Deserialize, Serialize)]
pub struct SubmitProofResp {
    /// submit result
    pub res: SubmitResult,

    /// description
    pub desc: Option<String>,
}

/// response for the poll_proof_state request
#[derive(Deserialize, Serialize)]
pub struct PollProofStateResp {
    /// on chain state
    pub state: OnChainState,

    /// description
    pub desc: Option<String>,
}

/// defines the SealerRpc service
#[rpc]
pub trait SealerRpc {
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
    ) -> Result<SubmitPreCommitResp>;

    /// api definition
    #[rpc(name = "Venus.PollPreCommitState")]
    fn poll_pre_commit_state(&self, id: SectorID) -> Result<PollPreCommitStateResp>;

    /// api definition
    #[rpc(name = "Venus.AssignSeed")]
    fn assign_seed(&self, id: SectorID) -> Result<Seed>;

    /// api definition
    #[rpc(name = "Venus.SubmitProof")]
    fn submit_proof(&self, id: SectorID, proof: ProofOnChainInfo) -> Result<SubmitProofResp>;

    /// api definition
    #[rpc(name = "Venus.PollProofState")]
    fn poll_proof_state(&self, id: SectorID) -> Result<PollProofStateResp>;
}
