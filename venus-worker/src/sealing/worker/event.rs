use std::fmt::{self, Debug};

use anyhow::{anyhow, Result};
use forest_address::Address;

use super::sector::{Base, Sector, State};
use crate::logging::trace;
use crate::rpc::sealer::{AllocatedSector, Deals, Seed, Ticket};
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
    SetState(State),

    Retry,

    Allocate(AllocatedSector),

    AcquireDeals(Option<Deals>),

    AddPiece(Vec<PieceInfo>),

    AssignTicket(Ticket),

    PC1(SealPreCommitPhase1Output),

    PC2(SealPreCommitPhase2Output),

    SubmitPC,

    CheckPC,

    Persist(String),

    SubmitPersistance,

    AssignSeed(Seed),

    C1(SealCommitPhase1Output),

    C2(SealCommitPhase2Output),

    SubmitProof,

    Finish,
}

impl Debug for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use Event::*;
        let name = match self {
            SetState(_) => "SetState",

            Retry => "Retry",

            Allocate(_) => "Allocate",

            AcquireDeals(_) => "AcquireDeals",

            AddPiece(_) => "AddPiece",

            AssignTicket(_) => "AssignTicket",

            PC1(_) => "PC1",

            PC2(_) => "PC2",

            SubmitPC => "SubmitPC",

            CheckPC => "CheckPC",

            Persist(_) => "Persist",

            SubmitPersistance => "SubmitPersistance",

            AssignSeed(_) => "AssignSeed",

            C1(_) => "C1",

            C2(_) => "C2",

            SubmitProof => "SubmitProof",

            Finish => "Finish",
        };

        f.write_str(name)
    }
}

macro_rules! replace {
    ($target:expr, $val:expr) => {
        let prev = $target.replace($val);
        if let Some(st) = $target.as_ref() {
            trace!("{:?} => {:?}", prev, st);
        }
    };
}

macro_rules! mem_replace {
    ($target:expr, $val:expr) => {
        let prev = std::mem::replace(&mut $target, $val);
        trace!("{:?} => {:?}", prev, $target);
    };
}

impl Event {
    pub fn apply(self, s: &mut Sector) -> Result<()> {
        let next = if let Event::SetState(s) = self {
            s
        } else {
            self.plan(&s.state)?
        };

        if next == s.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        self.apply_changes(s);
        s.update_state(next);

        Ok(())
    }

    fn apply_changes(self, s: &mut Sector) {
        use Event::*;
        match self {
            SetState(_) => {}

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

                replace!(s.base, base);
            }

            AcquireDeals(deals) => {
                mem_replace!(s.deals, deals);
            }

            AddPiece(pieces) => {
                replace!(s.phases.pieces, pieces);
            }

            AssignTicket(ticket) => {
                replace!(s.phases.ticket, ticket);
            }

            PC1(out) => {
                replace!(s.phases.pc1out, out);
            }

            PC2(out) => {
                replace!(s.phases.pc2out, out);
            }

            Persist(instance) => {
                replace!(s.phases.persist_instance, instance);
            }

            AssignSeed(seed) => {
                replace!(s.phases.seed, seed);
            }

            C1(out) => {
                replace!(s.phases.c1out, out);
            }

            C2(out) => {
                replace!(s.phases.c2out, out);
            }

            SubmitPC | CheckPC | SubmitPersistance | SubmitProof => {}

            Finish => {}
        };
    }

    fn plan(&self, st: &State) -> Result<State> {
        // syntax:
        // prev_state => {
        //      event0 => next_state0,
        //      event1 => next_state1,
        //      ...
        // },
        //
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
                Event::CheckPC => State::PCLanded,
            },

            State::PCLanded => {
                Event::Persist(_) => State::Persisted,
            },

            State::Persisted => {
                Event::SubmitPersistance => State::PersistanceSubmitted,
            },

            State::PersistanceSubmitted => {
                Event::AssignSeed(_) => State::SeedAssigned,
            },

            State::SeedAssigned => {
                Event::C1(_) => State::C1Done,
            },

            State::C1Done => {
                Event::C2(_) => State::C2Done,
            },

            State::C2Done => {
                Event::SubmitProof => State::ProofSubmitted,
            },


            State::ProofSubmitted => {
                Event::Finish => State::Finished,
            },
        };

        Ok(next)
    }
}
