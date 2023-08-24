use crate::rpc::sealer::{AllocatedSector, Deals, Seed, Ticket};
use crate::sealing::processor::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output, SealPreCommitPhase2Output, SectorId,
};
use serde::{Deserialize, Serialize};

use super::State;

#[derive(Deserialize, Serialize)]
pub struct Sectors {
    pub version: u32,

    pub state: State,
    pub prev_state: Option<State>,
    pub retry: u32,

    pub batch_size: usize,
    pub sectors: Vec<Sector>,
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
    pub state: State,
    pub base: Option<Base>,

    // deal pieces
    pub deals: Option<Deals>,
    pub phases: Phases,
}
