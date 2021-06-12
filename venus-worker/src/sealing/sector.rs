use forest_encoding::repr::{Deserialize_repr, Serialize_repr};
use serde::{Deserialize, Serialize};

const current_sector_version: u32 = 1;

pub use fil_clock::ChainEpoch;
pub use fil_types::{InteractiveSealRandomness, PieceInfo as DealInfo, Randomness, SectorID};
pub use filecoin_proofs_api::seal::{
    SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output,
};
pub use filecoin_proofs_api::{PieceInfo, RegisteredSealProof};

#[derive(Clone, Copy, Deserialize_repr, Serialize_repr, PartialEq)]
#[repr(u64)]
pub enum State {
    Empty,
    Allocated,
    DealsAcquired,
    PieceAdded,
    TicketAssigned,
    PC1Done,
    PC2Done,
    PCSubmitted,
    SeedAssigned,
    C1Done,
    C2Done,
    Persisted,
    ProofSubmitted,
    Finished,
}

impl std::fmt::Debug for State {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        let name = match self {
            State::Empty => "Empty",
            State::Allocated => "Allocated",
            State::DealsAcquired => "DealsAcquired",
            State::PieceAdded => "PieceAdded",
            State::TicketAssigned => "TicketAssigned",
            State::PC1Done => "PC1Done",
            State::PC2Done => "PC2Done",
            State::PCSubmitted => "PCSubmitted",
            State::SeedAssigned => "SeedAssigned",
            State::C1Done => "C1Done",
            State::C2Done => "C2Done",
            State::Persisted => "Persisted",
            State::ProofSubmitted => "ProofSubmitted",
            State::Finished => "Finished",
        };

        f.write_str(name)
    }
}

impl Default for State {
    fn default() -> Self {
        State::Empty
    }
}

#[derive(Deserialize, Serialize)]
pub struct Trace {
    pub prev: State,
    pub next: State,
    pub detail: String,
}

#[derive(Deserialize, Serialize)]
pub struct Ticket {
    pub ticket: Randomness,
    pub epoch: ChainEpoch,
}

#[derive(Deserialize, Serialize)]
pub struct Seed {
    pub seed: InteractiveSealRandomness,
    pub epoch: ChainEpoch,
}

#[derive(Default, Deserialize, Serialize)]
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

#[derive(Serialize, Deserialize)]
pub struct Base {
    // init infos
    pub id: SectorID,
    pub proof_type: RegisteredSealProof,
}

pub type Deals = Vec<DealInfo>;

#[derive(Serialize, Deserialize)]
pub struct Sector {
    pub version: u32,

    pub state: State,
    pub prev_state: Option<State>,
    pub retry: u32,

    pub base: Option<Base>,

    // deal pieces
    pub deals: Option<Deals>,

    pub phases: Phases,
}

impl Default for Sector {
    fn default() -> Self {
        Sector {
            version: current_sector_version,

            state: Default::default(),
            prev_state: None,
            retry: 0,

            base: None,

            deals: None,

            phases: Default::default(),
        }
    }
}
