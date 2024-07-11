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
#[allow(non_camel_case_types)]
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

    /// 2kib v1_1 with synthetic porep
    StackedDrg2KiBV1_1_Feat_SyntheticPoRep,
    /// 8mib v1_1 with synthetic porep
    StackedDrg8MiBV1_1_Feat_SyntheticPoRep,
    /// 512mib v1_1 with synthetic porep
    StackedDrg512MiBV1_1_Feat_SyntheticPoRep,
    /// 32gib v1_1 with synthetic porep
    StackedDrg32GiBV1_1_Feat_SyntheticPoRep,
    /// 64gib v1_1 with synthetic porep
    StackedDrg64GiBV1_1_Feat_SyntheticPoRep,

    /// 2kib v1_2 with niporep
    StackedDrg2KiBV1_2_Feat_NonInteractivePoRep,
    /// 8mib v1_2 with niporep
    StackedDrg8MiBV1_2_Feat_NonInteractivePoRep,
    /// 512mib v1_2 with niporep
    StackedDrg512MiBV1_2_Feat_NonInteractivePoRep,
    /// 32gib v1_2 with niporep
    StackedDrg32GiBV1_2_Feat_NonInteractivePoRep,
    /// 64gib v1_2 with niporep
    StackedDrg64GiBV1_2_Feat_NonInteractivePoRep,
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

            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep => SIZE_2K,
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep => SIZE_8M,
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep => SIZE_512M,
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep => SIZE_32G,
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep => SIZE_64G,

            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => SIZE_2K,
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => SIZE_8M,
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                SIZE_512M
            }
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => SIZE_32G,
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => SIZE_64G,
        }
    }

    /// returns post proof types for the seal proof type
    pub fn to_post_proofs(&self) -> Vec<RegisteredPoStProof> {
        use RegisteredPoStProof::*;
        use SealProof::*;
        match self {
            StackedDrg2KiBV1
            | StackedDrg2KiBV1_1
            | StackedDrg2KiBV1_1_Feat_SyntheticPoRep
            | StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => vec![
                StackedDrgWinning2KiBV1,
                StackedDrgWindow2KiBV1,
                StackedDrgWindow2KiBV1_2,
            ],
            StackedDrg8MiBV1
            | StackedDrg8MiBV1_1
            | StackedDrg8MiBV1_1_Feat_SyntheticPoRep
            | StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => vec![
                StackedDrgWinning8MiBV1,
                StackedDrgWindow8MiBV1,
                StackedDrgWindow8MiBV1_2,
            ],
            StackedDrg512MiBV1
            | StackedDrg512MiBV1_1
            | StackedDrg512MiBV1_1_Feat_SyntheticPoRep
            | StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                vec![
                    StackedDrgWinning512MiBV1,
                    StackedDrgWindow512MiBV1,
                    StackedDrgWindow512MiBV1_2,
                ]
            }
            StackedDrg32GiBV1
            | StackedDrg32GiBV1_1
            | StackedDrg32GiBV1_1_Feat_SyntheticPoRep
            | StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => vec![
                StackedDrgWinning32GiBV1,
                StackedDrgWindow32GiBV1,
                StackedDrgWindow32GiBV1_2,
            ],
            StackedDrg64GiBV1
            | StackedDrg64GiBV1_1
            | StackedDrg64GiBV1_1_Feat_SyntheticPoRep
            | StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => vec![
                StackedDrgWinning64GiBV1,
                StackedDrgWindow64GiBV1,
                StackedDrgWindow64GiBV1_2,
            ],
        }
    }

    /// transfer seal proof type to ni-porep proof type
    pub fn to_ni_porep(&self) -> Self {
        use RegisteredPoStProof::*;
        use SealProof::*;
        match self {
            StackedDrg2KiBV1
            | StackedDrg2KiBV1_1
            | StackedDrg2KiBV1_1_Feat_SyntheticPoRep
            | StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => {
                StackedDrg2KiBV1_2_Feat_NonInteractivePoRep
            }
            StackedDrg8MiBV1
            | StackedDrg8MiBV1_1
            | StackedDrg8MiBV1_1_Feat_SyntheticPoRep
            | StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => {
                StackedDrg8MiBV1_2_Feat_NonInteractivePoRep
            }
            StackedDrg512MiBV1
            | StackedDrg512MiBV1_1
            | StackedDrg512MiBV1_1_Feat_SyntheticPoRep
            | StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                StackedDrg512MiBV1_2_Feat_NonInteractivePoRep
            }
            StackedDrg32GiBV1
            | StackedDrg32GiBV1_1
            | StackedDrg32GiBV1_1_Feat_SyntheticPoRep
            | StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => {
                StackedDrg32GiBV1_2_Feat_NonInteractivePoRep
            }
            StackedDrg64GiBV1
            | StackedDrg64GiBV1_1
            | StackedDrg64GiBV1_1_Feat_SyntheticPoRep
            | StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => {
                StackedDrg64GiBV1_2_Feat_NonInteractivePoRep
            }
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

            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep
            }

            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep
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

            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep
            }
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredSealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep
            }

            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep
            }
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredSealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep
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

            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => {
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

            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }

            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg2KiBV1
            }
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg8MiBV1
            }
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg512MiBV1
            }
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg32GiBV1
            }
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep => {
                RegisteredUpdateProof::StackedDrg64GiBV1
            }
        }
    }
}

pub(crate) fn seal_types_from_u64(u: u64) -> Result<Vec<SealProof>, Error> {
    match u {
        SIZE_2K => Ok(vec![
            SealProof::StackedDrg2KiBV1_1,
            SealProof::StackedDrg2KiBV1_1_Feat_SyntheticPoRep,
            SealProof::StackedDrg2KiBV1_2_Feat_NonInteractivePoRep,
        ]),
        SIZE_8M => Ok(vec![
            SealProof::StackedDrg8MiBV1_1,
            SealProof::StackedDrg8MiBV1_1_Feat_SyntheticPoRep,
            SealProof::StackedDrg8MiBV1_2_Feat_NonInteractivePoRep,
        ]),
        SIZE_512M => Ok(vec![
            SealProof::StackedDrg512MiBV1_1,
            SealProof::StackedDrg512MiBV1_1_Feat_SyntheticPoRep,
            SealProof::StackedDrg512MiBV1_2_Feat_NonInteractivePoRep,
        ]),
        SIZE_32G => Ok(vec![
            SealProof::StackedDrg32GiBV1_1,
            SealProof::StackedDrg32GiBV1_1_Feat_SyntheticPoRep,
            SealProof::StackedDrg32GiBV1_2_Feat_NonInteractivePoRep,
        ]),
        SIZE_64G => Ok(vec![
            SealProof::StackedDrg64GiBV1_1,
            SealProof::StackedDrg64GiBV1_1_Feat_SyntheticPoRep,
            SealProof::StackedDrg64GiBV1_2_Feat_NonInteractivePoRep,
        ]),

        other => Err(anyhow!("invalid sector size {}", other)),
    }
}
