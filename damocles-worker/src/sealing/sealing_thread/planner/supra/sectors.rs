use crate::rpc::sealer::{AllocatedSector, Deals, SectorID, Seed, Ticket};
use crate::sealing::processor::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output,
    SealPreCommitPhase1Output, SealPreCommitPhase2Output, SectorId,
};
use crate::SealProof;
use serde::{Deserialize, Serialize};
use vc_processors::fil_proofs::{ActorID, Commitment};

use super::State;

const CURRENT_SECTOR_VERSION: u32 = 1;

#[derive(Deserialize, Serialize)]
pub struct Sectors {
    pub version: u32,

    pub state: State,
    pub prev_state: Option<State>,
    pub retry: u32,

    pub seal_proof: SealProof,
    pub block_offset: u64,
    pub num_sectors: usize,

    pub sectors: Vec<Sector>,
}

impl Sectors {
    pub fn new(block_offset: u64, num_sectors: usize) -> Self {
        Self {
            version: CURRENT_SECTOR_VERSION,
            state: State::Empty,
            prev_state: None,
            retry: 0,
            seal_proof: SealProof::StackedDrg32GiBV1_1,
            block_offset,
            num_sectors,
            sectors: Vec::new(),
        }
    }

    pub fn job_id(&self) -> Option<String> {
        match (self.sectors.first(), self.sectors.last()) {
            (Some(first), Some(last)) => {
                Some(if first.sector_id == last.sector_id {
                    first.sector_id.to_string()
                } else {
                    format!("{}..{}", first.sector_id, last.sector_id)
                })
            }

            _ => None,
        }
    }

    pub fn update_state(&mut self, next: State) {
        let prev = std::mem::replace(&mut self.state, next);
        self.prev_state.replace(prev);
    }
}

#[derive(Default, Deserialize, Serialize)]
pub struct Phases {
    // pc1
    pub pieces: Option<Vec<PieceInfo>>,
    pub ticket: Option<Ticket>,
    pub comm_d: Option<Commitment>,
    pub pc1_replica_id: Option<[u8; 32]>,

    // pc2
    pub pc2_re_submit: bool,
    pub comm_r: Option<Commitment>,
    pub persist_instance: Option<String>,
    pub pc2_landed: bool,

    // c1
    pub seed: Option<Seed>,
    pub c1out: Option<SealCommitPhase1Output>,

    // c2
    pub c2out: Option<SealCommitPhase2Output>,
    pub c2_re_submit: bool,
    pub c2_landed: bool,
}

#[derive(Serialize, Deserialize)]
pub struct Sector {
    pub prover_id: ProverId,
    pub sector_id: SectorID,

    pub aborted_reason: Option<String>,

    // deal pieces
    pub deals: Option<Deals>,
    pub phases: Phases,
}

impl Sector {
    pub fn new(prover_id: ProverId, sector_id: SectorID) -> Self {
        Self {
            prover_id,
            sector_id,
            aborted_reason: None,
            deals: None,
            phases: Phases {
                pieces: None,
                ticket: None,
                comm_d: None,
                pc1_replica_id: None,
                pc2_re_submit: false,
                pc2_landed: false,
                comm_r: None,
                persist_instance: None,
                seed: None,
                c1out: None,
                c2out: None,
                c2_re_submit: false,
                c2_landed: false,
            },
        }
    }

    pub fn is_cc(&self) -> bool {
        self.deals.as_ref().map(|d| d.len()).unwrap_or(0) == 0
    }

    pub fn aborted(&self) -> bool {
        self.aborted_reason.is_some()
    }
}
