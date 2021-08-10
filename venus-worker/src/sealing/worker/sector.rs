use serde::{Deserialize, Serialize};
use serde_repr::{Deserialize_repr, Serialize_repr};

pub use fil_clock::ChainEpoch;
pub use fil_types::{InteractiveSealRandomness, PieceInfo as DealInfo, Randomness};

use crate::rpc::{AllocatedSector, Deals, Seed, Ticket};
use crate::sealing::seal::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, SectorId,
};

const CURRENT_SECTOR_VERSION: u32 = 1;

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

#[derive(Debug, Deserialize, Serialize)]
pub struct Base {
    pub allocated: AllocatedSector,
    pub prove_input: (ProverId, SectorId),
}

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

impl Sector {
    pub fn update_state(&mut self, next: State) {
        let prev = std::mem::replace(&mut self.state, next);
        self.prev_state.replace(prev);
    }
}

impl Default for Sector {
    fn default() -> Self {
        Sector {
            version: CURRENT_SECTOR_VERSION,

            state: Default::default(),
            prev_state: None,
            retry: 0,

            base: None,

            deals: None,

            phases: Default::default(),
        }
    }
}
