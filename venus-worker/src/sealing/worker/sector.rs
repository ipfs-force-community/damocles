use anyhow::{anyhow, Error};
use serde::{Deserialize, Serialize};
use serde_repr::{Deserialize_repr, Serialize_repr};

pub use fil_clock::ChainEpoch;
pub use fil_types::{InteractiveSealRandomness, PieceInfo as DealInfo, Randomness};

use crate::rpc::sealer::{AllocatedSector, Deals, Seed, Ticket};
use crate::sealing::seal::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, SectorId,
};

const CURRENT_SECTOR_VERSION: u32 = 1;

macro_rules! def_state {
    ($($name:ident,)+) => {
        #[derive(Clone, Copy, Deserialize_repr, Serialize_repr, PartialEq)]
        #[repr(u64)]
        pub enum State {
            $(
                $name,
            )+
        }

        impl  State {
            pub fn as_str(&self) -> &'static str {
                match self {
                    $(
                        Self::$name => stringify!($name),
                    )+
                }
            }
        }

        impl Into<&str> for State {
            fn into(self) -> &'static str {
                self.as_str()
            }
        }

        impl std::str::FromStr for State {
            type Err = Error;
            fn from_str(s: &str) -> Result<Self, Self::Err> {
                match s {
                    $(
                        stringify!($name) => Ok(Self::$name),
                    )+

                    other => Err(anyhow!("invalid state {}", other)),
                }
            }
        }
    };
}

def_state! {
    Empty,
    Allocated,
    DealsAcquired,
    PieceAdded,
    TicketAssigned,
    PC1Done,
    PC2Done,
    PCSubmitted,
    PCLanded,
    Persisted,
    PersistanceSubmitted,
    SeedAssigned,
    C1Done,
    C2Done,
    ProofSubmitted,
    Finished,
}

impl std::fmt::Debug for State {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        f.write_str((*self).into())
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

    pub pc2_re_submit: bool,

    pub persist_instance: Option<String>,

    // c1
    pub seed: Option<Seed>,
    pub c1out: Option<SealCommitPhase1Output>,

    // c2
    pub c2out: Option<SealCommitPhase2Output>,

    pub c2_re_submit: bool,
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
