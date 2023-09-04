//! shared data types

use std::convert::TryFrom;

use anyhow::{anyhow, Error};
use serde_repr::{Deserialize_repr, Serialize_repr};
use vc_processors::fil_proofs::RegisteredPoStProof;

use crate::sealing::processor::{RegisteredSealProof, RegisteredUpdateProof};

pub const SIZE_2K: u64 = 2 << 10;
pub const SIZE_8M: u64 = 8 << 20;
pub const SIZE_512M: u64 = 512 << 20;
pub const SIZE_32G: u64 = 32 << 30;
pub const SIZE_64G: u64 = 64 << 30;

/// seal proof types with repr i64
#[derive(
    Clone, Copy, Debug, PartialEq, Eq, Hash, Deserialize_repr, Serialize_repr,
)]
#[repr(i64)]
pub enum SealProof {
    /// 2kib v1
    StackedDrg2KiBV1,

    /// 8mib v1
    StackedDrg8MiBV1,

    /// 512mib v1
    StackedDrg512MiBV1,

    /// 32gib v1
    StackedDrg32GiBV1,

    /// 64gib v1
    StackedDrg64GiBV1,

    /// 2kib v1_1
    StackedDrg2KiBV1_1,

    /// 8mib v1_1
    StackedDrg8MiBV1_1,

    /// 512mib v1_1
    StackedDrg512MiBV1_1,

    /// 32gib v1_1
    StackedDrg32GiBV1_1,

    /// 64gib v1_1
    StackedDrg64GiBV1_1,
}

impl SealProof {
    /// returns sector size for the seal proof type
    pub fn sector_size(&self) -> u64 {
        match self {
            SealProof::StackedDrg2KiBV1 => SIZE_2K,
            SealProof::StackedDrg8MiBV1 => SIZE_8M,
            SealProof::StackedDrg512MiBV1 => SIZE_512M,
            SealProof::StackedDrg32GiBV1 => SIZE_32G,
            SealProof::StackedDrg64GiBV1 => SIZE_64G,

            SealProof::StackedDrg2KiBV1_1 => SIZE_2K,
            SealProof::StackedDrg8MiBV1_1 => SIZE_8M,
            SealProof::StackedDrg512MiBV1_1 => SIZE_512M,
            SealProof::StackedDrg32GiBV1_1 => SIZE_32G,
            SealProof::StackedDrg64GiBV1_1 => SIZE_64G,
        }
    }

    /// returns post proof types for the seal proof type
    pub fn to_post_proofs(&self) -> Vec<RegisteredPoStProof> {
        use RegisteredPoStProof::*;
        use SealProof::*;
        match self {
            StackedDrg2KiBV1 | StackedDrg2KiBV1_1 => vec![
                StackedDrgWinning2KiBV1,
                StackedDrgWindow2KiBV1,
                StackedDrgWindow2KiBV1_2,
            ],
            StackedDrg8MiBV1 | StackedDrg8MiBV1_1 => vec![
                StackedDrgWinning8MiBV1,
                StackedDrgWindow8MiBV1,
                StackedDrgWindow8MiBV1_2,
            ],
            StackedDrg512MiBV1 | StackedDrg512MiBV1_1 => {
                vec![
                    StackedDrgWinning512MiBV1,
                    StackedDrgWindow512MiBV1,
                    StackedDrgWindow512MiBV1_2,
                ]
            }
            StackedDrg32GiBV1 | StackedDrg32GiBV1_1 => vec![
                StackedDrgWinning32GiBV1,
                StackedDrgWindow32GiBV1,
                StackedDrgWindow32GiBV1_2,
            ],
            StackedDrg64GiBV1 | StackedDrg64GiBV1_1 => vec![
                StackedDrgWinning64GiBV1,
                StackedDrgWindow64GiBV1,
                StackedDrgWindow64GiBV1_2,
            ],
        }
    }
}

impl TryFrom<u64> for SealProof {
    type Error = Error;

    fn try_from(val: u64) -> Result<Self, Self::Error> {
        match val {
            SIZE_2K => Ok(SealProof::StackedDrg2KiBV1_1),
            SIZE_8M => Ok(SealProof::StackedDrg8MiBV1_1),
            SIZE_512M => Ok(SealProof::StackedDrg512MiBV1_1),
            SIZE_32G => Ok(SealProof::StackedDrg32GiBV1_1),
            SIZE_64G => Ok(SealProof::StackedDrg64GiBV1_1),
            other => Err(anyhow!("invalid sector size {}", other)),
        }
    }
}

impl From<SealProof> for RegisteredSealProof {
    fn from(val: SealProof) -> Self {
        match val {
            SealProof::StackedDrg2KiBV1 => {
                RegisteredSealProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1 => {
                RegisteredSealProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1 => {
                RegisteredSealProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1 => {
                RegisteredSealProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1 => {
                RegisteredSealProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_1 => {
                RegisteredSealProof::StackedDrg2KiBV1_1
            }
            SealProof::StackedDrg8MiBV1_1 => {
                RegisteredSealProof::StackedDrg8MiBV1_1
            }
            SealProof::StackedDrg512MiBV1_1 => {
                RegisteredSealProof::StackedDrg512MiBV1_1
            }
            SealProof::StackedDrg32GiBV1_1 => {
                RegisteredSealProof::StackedDrg32GiBV1_1
            }
            SealProof::StackedDrg64GiBV1_1 => {
                RegisteredSealProof::StackedDrg64GiBV1_1
            }
        }
    }
}

impl From<&SealProof> for RegisteredSealProof {
    fn from(val: &SealProof) -> Self {
        match val {
            SealProof::StackedDrg2KiBV1 => {
                RegisteredSealProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1 => {
                RegisteredSealProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1 => {
                RegisteredSealProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1 => {
                RegisteredSealProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1 => {
                RegisteredSealProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_1 => {
                RegisteredSealProof::StackedDrg2KiBV1_1
            }
            SealProof::StackedDrg8MiBV1_1 => {
                RegisteredSealProof::StackedDrg8MiBV1_1
            }
            SealProof::StackedDrg512MiBV1_1 => {
                RegisteredSealProof::StackedDrg512MiBV1_1
            }
            SealProof::StackedDrg32GiBV1_1 => {
                RegisteredSealProof::StackedDrg32GiBV1_1
            }
            SealProof::StackedDrg64GiBV1_1 => {
                RegisteredSealProof::StackedDrg64GiBV1_1
            }
        }
    }
}

impl From<SealProof> for RegisteredUpdateProof {
    fn from(val: SealProof) -> Self {
        match val {
            SealProof::StackedDrg2KiBV1 => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1 => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1 => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1 => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1 => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_1 => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_1 => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_1 => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_1 => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_1 => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }
        }
    }
}

impl From<&SealProof> for RegisteredUpdateProof {
    fn from(val: &SealProof) -> Self {
        match val {
            SealProof::StackedDrg2KiBV1 => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1 => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1 => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1 => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1 => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_1 => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_1 => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_1 => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_1 => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_1 => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }
        }
    }
}
