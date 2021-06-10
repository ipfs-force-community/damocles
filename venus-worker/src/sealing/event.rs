use std::fmt::{self, Debug};

use anyhow::{anyhow, Result};

use super::sector::{
    Base, Deals, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output,
    SealPreCommitPhase2Output, Sector, Seed, State, Ticket,
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
    Allocate(Base),

    AcquireDeals(Option<Deals>),

    AddPiece,

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
            Allocate(_) => "Allocate",

            AcquireDeals(_) => "AcquireDeals",

            AddPiece => "AddPiece",

            _ => unreachable!(),
        };

        f.write_str(name)
    }
}

impl Event {
    pub fn apply(self, s: &mut Sector) -> Result<()> {
        let next = self.plan(&s.state)?;
        if next == s.state {
            return Err(anyhow!(
                "state({:?}) unchanged, may be an infinite loop",
                next
            ));
        }

        self.apply_changes(s);
        let prev = std::mem::replace(&mut s.state, next);
        s.prev_state.replace(prev);

        Ok(())
    }

    fn apply_changes(self, s: &mut Sector) {
        use Event::*;
        match self {
            Allocate(base) => {
                s.base.replace(base);
            }

            AcquireDeals(deals) => {
                std::mem::replace(&mut s.deals, deals).map(drop);
            }

            AddPiece => {}

            _ => unreachable!(),
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
                Event::AddPiece => State::PieceAdded,
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
