use std::fmt::{self, Debug};

use anyhow::{anyhow, Result};
use forest_address::Address;

use super::sector::{Base, Sector, State};
use crate::rpc::{AllocatedSector, Deals, Seed, Ticket};
use crate::sealing::seal::{
    PieceInfo, ProverId, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, SectorId,
};

macro_rules! plan {
    ($e:expr, $st:expr, $($prev:pat => {$($evt:pat => $next:expr,)+},)*) => {
        match $st {
            $(
                $prev => {
                    match $e {
                        $(
                            $evt => $next,
                            _ => return Err(anyhow!("unexpected event {:?} for state {:?}", $e, $st)),
                        )+
                    }
                }
            )*

            other => return Err(anyhow!("unexpected state {:?}", other)),
        }
    };
}

pub enum Event {
    Retry,

    Allocate(AllocatedSector),

    AcquireDeals(Option<Deals>),

    AddPiece(Vec<PieceInfo>),

    AssignTicket(Ticket),

    PC1(SealPreCommitPhase1Output),

    PC2(SealPreCommitPhase2Output),

    SubmitPC,

    AssignSeed(Seed),

    C1(SealCommitPhase1Output),

    C2(SealCommitPhase2Output),

    Persist,

    SubmitProof,

    Finish,
}

impl Debug for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use Event::*;
        let name = match self {
            Retry => "Retry",

            Allocate(_) => "Allocate",

            AcquireDeals(_) => "AcquireDeals",

            AddPiece(_) => "AddPiece",

            AssignTicket(_) => "AssignTicket",

            PC1(_) => "PC1",

            PC2(_) => "PC2",

            SubmitPC => "SubmitPC",

            AssignSeed(_) => "AssignSeed",

            C1(_) => "C1",

            C2(_) => "C2",

            Persist => "Persist",

            SubmitProof => "SubmitProof",

            Finish => "Finish",
        };

        f.write_str(name)
    }
}

impl Event {
    pub fn apply(self, s: &mut Sector) -> Result<()> {
        let next = self.plan(&s.state)?;
        if next == s.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        self.apply_changes(s);
        let prev = std::mem::replace(&mut s.state, next);
        s.prev_state.replace(prev);

        Ok(())
    }

    fn apply_changes(self, s: &mut Sector) {
        use Event::*;
        match self {
            Retry => {}

            Allocate(sector) => {
                let mut prover_id: ProverId = Default::default();
                let actor_addr_payload = Address::new_id(sector.id.miner).payload_bytes();
                prover_id[..actor_addr_payload.len()].copy_from_slice(actor_addr_payload.as_ref());

                let sector_id = SectorId::from(sector.id.number);

                let base = Base {
                    allocated: sector,
                    prove_input: (prover_id, sector_id),
                };

                s.base.replace(base);
            }

            AcquireDeals(deals) => {
                std::mem::replace(&mut s.deals, deals).map(drop);
            }

            AddPiece(pieces) => {
                s.phases.pieces.replace(pieces);
            }

            AssignTicket(ticket) => {
                s.phases.ticket.replace(ticket);
            }

            PC1(out) => {
                s.phases.pc1out.replace(out);
            }

            PC2(out) => {
                s.phases.pc2out.replace(out);
            }

            AssignSeed(seed) => {
                s.phases.seed.replace(seed);
            }

            C1(out) => {
                s.phases.c1out.replace(out);
            }

            C2(out) => {
                s.phases.c2out.replace(out);
            }

            SubmitPC | Persist | SubmitProof => {}

            Finish => {}
        };
    }

    fn plan(&self, st: &State) -> Result<State> {
        let next = plan! {
            self,
            st,

            State::Empty => {
                Event::Allocate(_) => State::Allocated,
            },

            State::Allocated => {
                Event::AcquireDeals(_) => State::DealsAcquired,
            },

            State::DealsAcquired => {
                Event::AddPiece(_) => State::PieceAdded,
            },

            State::PieceAdded => {
                Event::AssignTicket(_) => State::TicketAssigned,
            },

            State::TicketAssigned => {
                Event::PC1(_) => State::PC1Done,
            },

            State::PC1Done => {
                Event::PC2(_) => State::PC2Done,
            },

            State::PC2Done => {
                Event::SubmitPC => State::PCSubmitted,
            },

            State::PCSubmitted => {
                Event::AssignSeed(_) => State::SeedAssigned,
            },

            State::SeedAssigned => {
                Event::C1(_) => State::C1Done,
            },

            State::C1Done => {
                Event::C2(_) => State::C2Done,
            },

            State::C2Done => {
                Event::Persist => State::Persisted,
            },

            State::Persisted => {
                Event::SubmitProof => State::ProofSubmitted,
            },

            State::ProofSubmitted => {
                Event::Finish => State::Finished,
            },
        };

        Ok(next)
    }
}
