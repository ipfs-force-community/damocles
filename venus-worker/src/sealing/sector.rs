use fil_types::{PieceInfo as DealInfo, SectorID};
use filecoin_proofs_api::{
    seal::{
        SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
        SealPreCommitPhase2Output,
    },
    PieceInfo, RegisteredSealProof, Ticket as Randomness,
};

pub enum State {
    Unknown,
}

pub struct Ticket {
    pub ticket: Randomness,
    pub epoch: i64,
}

pub struct Seed {
    pub seed: Randomness,
    pub epoch: i64,
}

#[derive(Default)]
pub struct Phases {
    // pc1
    pub pieces: Option<Vec<PieceInfo>>,
    pub ticket: Option<Ticket>,
    pub pc1out: Option<SealPreCommitPhase1Output>,

    // pc2
    pub pc2out: Option<SealPreCommitPhase2Output>,

    // c1
    pub seed: Option<Seed>,
    pub c1out: Option<SealCommitPhase1Output>,

    // c2
    pub c2out: Option<SealCommitPhase2Output>,
}

pub struct Sector {
    pub state: State,
    pub prev_state: Option<State>,

    // init infos
    pub id: SectorID,
    pub proof_type: RegisteredSealProof,

    // deal pieces
    pub deals: Option<Vec<DealInfo>>,

    pub phases: Phases,
}

pub struct Trace {
    pub prev: State,
    pub next: State,
    pub detail: String,
}
