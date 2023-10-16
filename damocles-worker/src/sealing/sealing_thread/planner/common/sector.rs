use anyhow::{anyhow, Error};
use forest_cid::json::CidJson;
use serde::{Deserialize, Serialize};
use serde_repr::{Deserialize_repr, Serialize_repr};

pub use fil_clock::ChainEpoch;
pub use fil_types::{
    InteractiveSealRandomness, PieceInfo as DealInfo, Randomness,
};

use crate::rpc::sealer::{
    AllocatedSector, Deals, SectorPrivateInfo, SectorPublicInfo, Seed, Ticket,
};
use crate::sealing::processor::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output,
    SealPreCommitPhase1Output, SealPreCommitPhase2Output, SectorId,
    SnapEncodeOutput,
};
use crate::sealing::sealing_thread::default_plan;
use crate::sealing::sealing_thread::planner::{
    PLANNER_NAME_REBUILD, PLANNER_NAME_SNAPUP, PLANNER_NAME_UNSEAL,
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

        impl State {
            pub fn as_str(&self) -> &'static str {
                match self {
                    $(
                        Self::$name => stringify!($name),
                    )+
                }
            }
        }

        impl From<State> for &str {
            fn from(s: State) -> &'static str {
                s.as_str()
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

impl State {
    pub fn stage(&self, plan: impl AsRef<str>) -> &'static str {
        match (self, plan.as_ref()) {
            (State::Empty, _) => "-",
            (State::Allocated, PLANNER_NAME_SNAPUP)
            | (State::Allocated, PLANNER_NAME_REBUILD) => "AddPieces",
            (State::Allocated, PLANNER_NAME_UNSEAL) => "Unseal",
            (State::Allocated, _) => "AcquireDeals",
            (State::DealsAcquired, _) => "AddPieces",
            (State::PieceAdded, _) => "TreeD",
            (State::TreeDBuilt, PLANNER_NAME_REBUILD) => "PC1",
            (State::TreeDBuilt, PLANNER_NAME_SNAPUP) => "SnapEncode",
            (State::TreeDBuilt, _) => "AssignTicket",
            (State::TicketAssigned, _) => "PC1",
            (State::PC1Done, _) => "PC2",
            (State::SyntheticPoRepNeeded, _) => "PC2",
            (State::PC2Done, PLANNER_NAME_REBUILD) => "C1",
            (State::PC2Done, _) => "SubmitPreCommit",
            (State::PCSubmitted, _) => "WaitPCLanded",
            (State::PCLanded, _) | (State::SnapProved, _) => "PersistFiles",
            (State::Persisted, PLANNER_NAME_SNAPUP) => "SubmitSnapupProof",
            (State::Persisted, _) => "SubmitPersisted",
            (State::PersistanceSubmitted, _) => "WaitSeed",
            (State::SeedAssigned, _) => "C1",
            (State::C1Done, _) => "C2",
            (State::C2Done, _) => "SubmitProof",
            (State::ProofSubmitted, _) => "WaitProofLanded",
            (State::Finished, _) => "-",
            (State::Aborted, _) => "-",
            (State::SnapEncoded, _) => "SnapProve",
            (State::SealedChecked, PLANNER_NAME_REBUILD) => "PrepareForSnapup",
            (State::SnapPieceAdded, _) => "TreeD",
            (State::SnapTreeDBuilt, _) => "SnapEncode",
            (State::SnapDone, _) => "PersistFiles",
            (State::Unsealed, _) => "UploadPiece",
            _ => "-",
        }
    }
}

def_state! {
    Empty,
    Allocated,
    DealsAcquired,
    PieceAdded,
    TreeDBuilt,
    TicketAssigned,
    PC1Done,
    PC2Done,
    SyntheticPoRepNeeded,
    PCSubmitted,
    PCLanded,
    Persisted,
    PersistanceSubmitted,
    SeedAssigned,
    C1Done,
    C2Done,
    ProofSubmitted,
    Finished,
    Aborted,
    SnapEncoded,
    SnapProved,
    SealedChecked,
    SnapPieceAdded,
    SnapTreeDBuilt,
    SnapDone,
    Unsealed,
}

impl std::fmt::Debug for State {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        f.write_str((*self).into())
    }
}

#[allow(clippy::derivable_impls)]
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
pub struct UnsealInput {
    pub piece_cid: CidJson,
    pub comm_d: [u8; 32],
    pub offset: u64,
    pub size: u64,
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

    // snap up
    pub snap_encode_out: Option<SnapEncodeOutput>,
    pub snap_prov_out: Option<Vec<u8>>,

    // unseal
    pub unseal_in: Option<UnsealInput>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct Base {
    pub allocated: AllocatedSector,
    pub prove_input: (ProverId, SectorId),
}

#[derive(Debug, Deserialize, Serialize)]
pub struct Finalized {
    pub public: SectorPublicInfo,
    pub private: SectorPrivateInfo,
}

#[derive(Serialize, Deserialize)]
pub struct Sector {
    pub version: u32,

    pub plan: Option<String>,

    pub state: State,
    pub prev_state: Option<State>,
    pub retry: u32,

    pub base: Option<Base>,

    // deal pieces
    pub deals: Option<Deals>,

    // this field should only be set when the snapup procedures are required by the sector,
    // no matter it is a snapup- or rebuild- sector
    pub finalized: Option<Finalized>,

    pub phases: Phases,
}

impl Sector {
    pub fn new(plan: String) -> Self {
        Self {
            version: CURRENT_SECTOR_VERSION,
            plan: Some(plan),
            state: Default::default(),
            prev_state: None,
            retry: 0,
            base: None,
            deals: None,
            finalized: None,
            phases: Default::default(),
        }
    }

    pub fn update_state(&mut self, next: State) {
        let prev = std::mem::replace(&mut self.state, next);
        self.prev_state.replace(prev);
    }

    pub fn plan(&self) -> &str {
        self.plan.as_deref().unwrap_or_else(|| default_plan())
    }
}
