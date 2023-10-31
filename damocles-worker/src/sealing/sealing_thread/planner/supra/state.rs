use std::{
    fmt,
    ops::{Range, RangeBounds},
    str::FromStr,
};

use anyhow::{anyhow, Context, Error};
use once_cell::sync::Lazy;
use regex::Regex;
use serde::{Deserialize, Serialize};

use super::sectors::Sector;

#[derive(Debug, Serialize, Deserialize, Clone, Eq, PartialEq)]
pub enum State {
    Empty,
    Allocated,
    DealsAcquired { start_slot: usize, end_slot: usize },
    PieceAdded { start_slot: usize, end_slot: usize },
    TreeDBuilt { start_slot: usize, end_slot: usize },
    TicketAssigned { start_slot: usize, end_slot: usize },
    PC1Done,
    PC2Done,
    PCSubmitted { start_slot: usize, end_slot: usize },
    PCLanded { start_slot: usize, end_slot: usize },
    Persisted { start_slot: usize, end_slot: usize },
    PersistanceSubmitted { start_slot: usize, end_slot: usize },
    SeedAssigned { start_slot: usize, end_slot: usize },
    C1Done,
    C2Done { slot: usize },
    ProofSubmitted { start_slot: usize, end_slot: usize },
    Finished { start_slot: usize, end_slot: usize },
    Aborted,
}

impl State {
    pub fn pure(&self) -> String {
        let mut s = self.to_string();

        if let Some(new_len) = s.find("[") {
            s.truncate(new_len)
        }

        s
    }

    pub fn range<'a>(&self, sectors: &'a [Sector]) -> &'a [Sector] {
        use State::*;
        match *self {
            Empty | Allocated | PC1Done | PC2Done | C1Done | Aborted => {
                &sectors[..]
            }
            DealsAcquired {
                start_slot,
                end_slot,
            }
            | PieceAdded {
                start_slot,
                end_slot,
            }
            | TreeDBuilt {
                start_slot,
                end_slot,
            }
            | TicketAssigned {
                start_slot,
                end_slot,
            }
            | PCSubmitted {
                start_slot,
                end_slot,
            }
            | PCLanded {
                start_slot,
                end_slot,
            }
            | Persisted {
                start_slot,
                end_slot,
            }
            | PersistanceSubmitted {
                start_slot,
                end_slot,
            }
            | SeedAssigned {
                start_slot,
                end_slot,
            }
            | ProofSubmitted {
                start_slot,
                end_slot,
            }
            | Finished {
                start_slot,
                end_slot,
            } => &sectors[start_slot..end_slot],
            C2Done { slot } => &sectors[slot..slot + 1],
        }
    }
}

impl fmt::Display for State {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use State::*;

        match *self {
            Empty => write!(f, "Empty"),
            Allocated => write!(f, "Allocated"),
            DealsAcquired {
                start_slot,
                end_slot,
            } => write!(f, "DealsAcquired[{}:{}]", start_slot, end_slot),
            PieceAdded {
                start_slot,
                end_slot,
            } => write!(f, "PieceAdded[{}:{}]", start_slot, end_slot),
            TreeDBuilt {
                start_slot,
                end_slot,
            } => write!(f, "TreeDBuilt[{}:{}]", start_slot, end_slot),
            TicketAssigned {
                start_slot,
                end_slot,
            } => {
                write!(f, "TicketAssigned[{}:{}]", start_slot, end_slot)
            }
            PC1Done => write!(f, "PC1Done"),
            PC2Done => write!(f, "PC2Done"),
            PCSubmitted {
                start_slot,
                end_slot,
            } => write!(f, "PCSubmitted[{}:{}]", start_slot, end_slot),
            PCLanded {
                start_slot,
                end_slot,
            } => write!(f, "PCLanded[{}:{}]", start_slot, end_slot),
            Persisted {
                start_slot,
                end_slot,
            } => write!(f, "Persisted[{}:{}]", start_slot, end_slot),
            PersistanceSubmitted {
                start_slot,
                end_slot,
            } => write!(f, "PersistanceSubmitted[{}:{}]", start_slot, end_slot),
            SeedAssigned {
                start_slot,
                end_slot,
            } => write!(f, "SeedAssigned[{}:{}]", start_slot, end_slot),
            C1Done => write!(f, "C1Done"),
            C2Done { slot } => write!(f, "C2Done[{}]", slot),
            ProofSubmitted {
                start_slot,
                end_slot,
            } => write!(f, "ProofSubmitted[{}:{}]", start_slot, end_slot),
            Finished {
                start_slot,
                end_slot,
            } => write!(f, "Finished[{}:{}]", start_slot, end_slot),
            Aborted => write!(f, "Aborted"),
        }
    }
}

impl FromStr for State {
    type Err = Error;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        static STATE_RE: Lazy<Regex> = Lazy::new(|| {
            Regex::new(r"(\w+)(?:\[(\d{1,3})(?::(\d{1,3}))?\])?").unwrap()
        });

        enum StateShape {
            Slice { start_slot: usize, end_slot: usize },
            Index { slot: usize },
            Scalar,
        }

        let cap = STATE_RE
            .captures(s)
            .with_context(|| format!("invalid state: {}", s))?;

        let state = &cap[1];

        let state_shape = match (cap.get(2), cap.get(3)) {
            (Some(start_slot), Some(end_slot)) => {
                // MyState[10:100]
                StateShape::Slice {
                    start_slot: start_slot
                        .as_str()
                        .parse()
                        .expect("validated by regex"),
                    end_slot: end_slot
                        .as_str()
                        .parse()
                        .expect("validated by regex"),
                }
            }

            (Some(slot), None) => {
                // MyState[100]
                StateShape::Index {
                    slot: slot.as_str().parse().expect("validated by regex"),
                }
            }
            _ => StateShape::Scalar, // MyState
        };

        match (state, state_shape) {
            ("Empty", StateShape::Scalar) => Ok(State::Empty),
            ("Allocated", StateShape::Scalar) => Ok(State::Allocated),
            (
                "DealsAcquired",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::DealsAcquired {
                start_slot,
                end_slot,
            }),
            (
                "PieceAdded",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::PieceAdded {
                start_slot,
                end_slot,
            }),
            (
                "TreeDBuilt",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::TreeDBuilt {
                start_slot,
                end_slot,
            }),
            (
                "TicketAssigned",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::TicketAssigned {
                start_slot,
                end_slot,
            }),
            ("PC1Done", StateShape::Scalar) => Ok(State::PC1Done),
            ("PC2Done", StateShape::Scalar) => Ok(State::PC2Done),
            (
                "PCSubmitted",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::PCSubmitted {
                start_slot,
                end_slot,
            }),
            (
                "PCLanded",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::PCLanded {
                start_slot,
                end_slot,
            }),
            (
                "Persisted",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::Persisted {
                start_slot,
                end_slot,
            }),
            (
                "PersistanceSubmitted",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::PersistanceSubmitted {
                start_slot,
                end_slot,
            }),
            (
                "SeedAssigned",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::SeedAssigned {
                start_slot,
                end_slot,
            }),
            ("C1Done", StateShape::Scalar) => Ok(State::C1Done),
            ("C2Done", StateShape::Index { slot }) => {
                Ok(State::C2Done { slot })
            }
            (
                "ProofSubmitted",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::ProofSubmitted {
                start_slot,
                end_slot,
            }),
            (
                "Finished",
                StateShape::Slice {
                    start_slot,
                    end_slot,
                },
            ) => Ok(State::Finished {
                start_slot,
                end_slot,
            }),
            ("Aborted", StateShape::Scalar) => Ok(State::C1Done),
            _ => Err(anyhow!("invalid state: {}", state)),
        }
    }
}
