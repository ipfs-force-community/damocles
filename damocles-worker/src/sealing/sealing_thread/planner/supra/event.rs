use core::fmt;

use anyhow::{anyhow, Result};
use vc_processors::fil_proofs::{
    to_prover_id, Commitment, PieceInfo, RegisteredSealProof,
    SealCommitPhase1Output, SealCommitPhase2Output,
};

use crate::{
    metadb::MaybeDirty,
    rpc::sealer::{Deals, SectorID, Seed, Ticket},
    SealProof,
};

use super::{
    generate_replica_id,
    sectors::{Sector, Sectors},
    Job, State,
};

#[derive(Clone)]
pub enum Event {
    SetState(State),
    // No specified tasks available from sector_manager.
    Idle,
    // If `Planner::exec` returns `Event::Retry` it will be retried indefinitely
    // until `Planner::exec` returns another `Event` or an error occurs
    #[allow(dead_code)]
    Retry,
    Allocate {
        seal_proof: SealProof,
        sectors: Vec<SectorID>,
    },
    AcquireDeals {
        start_slot: usize,
        end_slot: usize,
        chunk_deals: Vec<Option<Deals>>,
    },
    AddPiece {
        start_slot: usize,
        end_slot: usize,
        chunk_pieces: Vec<Vec<PieceInfo>>,
    },
    BuildTreeD {
        start_slot: usize,
        end_slot: usize,
        chunk_comm_d: Vec<Commitment>,
    },
    AssignTicket {
        start_slot: usize,
        end_slot: usize,
        chunk_ticket: Vec<Ticket>,
    },
    PC1,
    PC2(Vec<Commitment>), // comm_r
    SubmitPC {
        start_slot: usize,
        end_slot: usize,
    },
    ReSubmitPC {
        start_slot: usize,
        end_slot: usize,
    },
    CheckPC {
        start_slot: usize,
        end_slot: usize,
    },
    Persist {
        start_slot: usize,
        end_slot: usize,
        chunk_instance: Vec<String>,
    },
    SubmitPersistance {
        start_slot: usize,
        end_slot: usize,
    },
    AssignSeed {
        start_slot: usize,
        end_slot: usize,
        chunk_seed: Vec<Seed>,
    },
    C1(Vec<SealCommitPhase1Output>),
    C2 {
        slot: usize,
        out: SealCommitPhase2Output,
    },
    SubmitProof {
        start_slot: usize,
        end_slot: usize,
    },
    ReSubmitProof {
        start_slot: usize,
        end_slot: usize,
    },
    Finish {
        start_slot: usize,
        end_slot: usize,
    },
}

impl fmt::Debug for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self)
    }
}

impl fmt::Display for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use Event::*;

        match self {
            SetState(_) => write!(f, "SetState"),
            Idle => write!(f, "Idle"),
            Retry => write!(f, "Retry"),
            Allocate { .. } => write!(f, "Allocate"),
            AcquireDeals { .. } => write!(f, "AcquireDeals"),
            AddPiece { .. } => write!(f, "AddPiece"),
            BuildTreeD { .. } => write!(f, "BuildTreeD"),
            AssignTicket { .. } => write!(f, "AssignTicket"),
            PC1 => write!(f, "PC1"),
            PC2(_) => write!(f, "PC2"),
            SubmitPC { .. } => write!(f, "SubmitPC"),
            ReSubmitPC { .. } => write!(f, "ReSubmitPC"),
            CheckPC { .. } => write!(f, "CheckPC"),
            Persist { .. } => write!(f, "Persist"),
            SubmitPersistance { .. } => write!(f, "SubmitPersistance"),
            AssignSeed { .. } => write!(f, "AssignSeed"),
            C1(_) => write!(f, "C1"),
            C2 { .. } => write!(f, "C2"),
            SubmitProof { .. } => write!(f, "SubmitProof"),
            ReSubmitProof { .. } => write!(f, "ReSubmitProof"),
            Finish { .. } => write!(f, "Finish"),
        }
    }
}

impl Event {
    pub(crate) fn apply(self, state: State, job: &mut Job) -> Result<()> {
        let next = if let Event::SetState(s) = &self {
            s.clone()
        } else {
            state
        };

        if next == job.sectors.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        self.apply_changes(job.sectors.inner_mut());
        job.sectors.update_state(next);

        Ok(())
    }

    fn apply_changes(self, s: &mut MaybeDirty<Sectors>) {
        match self {
            Self::Allocate {
                seal_proof,
                sectors,
            } => {
                s.seal_proof = seal_proof;
                for allocated in sectors {
                    s.sectors.push(Sector::new(
                        to_prover_id(allocated.miner),
                        allocated,
                    ))
                }
            }

            Self::AcquireDeals {
                start_slot,
                end_slot,
                chunk_deals,
            } => {
                for (slot, deals) in (start_slot..end_slot).zip(chunk_deals) {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.deals = deals;
                    }
                }
            }

            Self::AddPiece {
                start_slot,
                end_slot,
                chunk_pieces,
            } => {
                for (slot, pieces) in (start_slot..end_slot).zip(chunk_pieces) {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.pieces.replace(pieces);
                    }
                }
            }

            Self::BuildTreeD {
                start_slot,
                end_slot,
                chunk_comm_d,
            } => {
                for (slot, comm_d) in (start_slot..end_slot).zip(chunk_comm_d) {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.comm_d.replace(comm_d);
                    }
                }
            }

            Self::AssignTicket {
                start_slot,
                end_slot,
                chunk_ticket,
            } => {
                let registered_seal_proof: RegisteredSealProof =
                    s.seal_proof.into();
                let porep_id = registered_seal_proof.as_v1_config().porep_id;

                for (slot, ticket) in (start_slot..end_slot).zip(chunk_ticket) {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        let comm_d =
                            sector.phases.comm_d.expect("comm_d required");
                        let replica_id = generate_replica_id(
                            &sector.prover_id,
                            sector.sector_id.number,
                            &ticket.ticket.0,
                            comm_d,
                            &porep_id,
                        );

                        sector.phases.ticket.replace(ticket);
                        sector.phases.pc1_replica_id.replace(replica_id);
                    }
                }
            }
            Self::PC1 => {}
            Self::PC2(all_comm_r) => {
                for (sector, comm_r) in s.sectors.iter_mut().zip(all_comm_r) {
                    sector.phases.comm_r.replace(comm_r);
                }
            }
            Self::Persist {
                start_slot,
                end_slot,
                chunk_instance,
            } => {
                for (slot, instance) in
                    (start_slot..end_slot).zip(chunk_instance)
                {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.persist_instance.replace(instance);
                    }
                }
            }

            Self::AssignSeed {
                start_slot,
                end_slot,
                chunk_seed,
            } => {
                for (slot, seed) in (start_slot..end_slot).zip(chunk_seed) {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.seed.replace(seed);
                    }
                }
            }

            Self::C1(out) => {
                for (sector, c1out) in s.sectors.iter_mut().zip(out) {
                    sector.phases.c1out.replace(c1out);
                }
            }

            Self::C2 { slot, out } => {
                if let Some(sector) = s.sectors.get_mut(slot) {
                    sector.phases.c2out.replace(out);
                }
            }

            Self::SubmitPC {
                start_slot,
                end_slot,
            } => {
                for slot in start_slot..end_slot {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.pc2_re_submit = false
                    }
                }
            }

            Self::ReSubmitPC {
                start_slot,
                end_slot,
            } => {
                for slot in start_slot..end_slot {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.pc2_re_submit = true
                    }
                }
            }

            Self::SubmitProof {
                start_slot,
                end_slot,
            } => {
                for slot in start_slot..end_slot {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.c2_re_submit = false
                    }
                }
            }

            Self::ReSubmitProof {
                start_slot,
                end_slot,
            } => {
                for slot in start_slot..end_slot {
                    if let Some(sector) = s.sectors.get_mut(slot) {
                        sector.phases.c2_re_submit = true
                    }
                }
            }
            _ => {}
        };
    }
}
