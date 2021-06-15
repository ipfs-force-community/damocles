use fil_clock::ChainEpoch;
use fil_types::{ActorID, InteractiveSealRandomness, PieceInfo, Randomness, SectorID};
use filecoin_proofs_api::{
    seal::{SealCommitPhase2Output, SealPreCommitPhase2Output},
    RegisteredSealProof,
};
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
pub struct AllocateSectorSpec {
    pub allowed_miners: Option<Vec<ActorID>>,
    pub allowed_proot_types: Option<Vec<RegisteredSealProof>>,
}

#[derive(Serialize, Deserialize)]
pub struct AllocatedSector {
    pub id: SectorID,
    pub proof_type: RegisteredSealProof,
}

pub type Deals = Vec<PieceInfo>;

#[derive(Serialize, Deserialize)]
pub struct AcquireDealsSpec {
    pub max_deals: Option<usize>,
}

#[derive(Deserialize, Serialize)]
pub struct Ticket {
    pub ticket: Randomness,
    pub epoch: ChainEpoch,
}

#[derive(Deserialize, Serialize)]
pub struct SubmitPreCommitReq {
    pub id: SectorID,
    pub out: SealPreCommitPhase2Output,
}

#[derive(Deserialize, Serialize)]
pub struct SubmitPreCommitResp {}

#[derive(Deserialize, Serialize)]
pub struct PollPreCommitStateReq {}

#[derive(Deserialize, Serialize)]
pub struct PollPreCommitStateResp {}

#[derive(Deserialize, Serialize)]
pub struct Seed {
    pub seed: InteractiveSealRandomness,
    pub epoch: ChainEpoch,
}

#[derive(Deserialize, Serialize)]
pub struct SubmitProofReq {
    pub id: SectorID,
    pub out: SealCommitPhase2Output,
}

#[derive(Deserialize, Serialize)]
pub struct SubmitProofResp {}

#[derive(Deserialize, Serialize)]
pub struct PollProofStateReq {}

#[derive(Deserialize, Serialize)]
pub struct PollProofStateResp {}

#[rpc(client)]
pub trait SealerRpc {
    #[rpc(name = "Venus.AllocateSector")]
    fn allocate_sector(&self, spec: AllocateSectorSpec) -> Result<Option<AllocatedSector>>;

    #[rpc(name = "Venus.AcquireDeals")]
    fn acquire_deals(&self, spec: AcquireDealsSpec) -> Result<Option<Deals>>;

    #[rpc(name = "Venus.AssignTicket")]
    fn assign_ticket(&self, id: SectorID) -> Result<Ticket>;

    #[rpc(name = "Venus.SubmitPreCommit")]
    fn submit_pre_commit(&self, req: SubmitPreCommitReq) -> Result<SubmitPreCommitResp>;

    #[rpc(name = "Venus.PollPreCommitState")]
    fn poll_pre_commit_state(&self, req: PollPreCommitStateReq) -> Result<PollPreCommitStateResp>;

    #[rpc(name = "Venus.AssignSeed")]
    fn assign_seed(&self, id: SectorID) -> Result<Seed>;

    #[rpc(name = "Venus.SubmitProof")]
    fn submit_proof(&self, req: SubmitProofReq) -> Result<SubmitProofResp>;

    #[rpc(name = "Venus.PollProofState")]
    fn poll_proof_state(&self, req: PollProofStateReq) -> Result<PollProofStateResp>;
}
