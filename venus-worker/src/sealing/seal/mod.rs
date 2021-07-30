//! processor abstractions & implementations for sealing

use std::fmt::Debug;
use std::path::PathBuf;

use anyhow::Result;
use serde::{de::DeserializeOwned, Deserialize, Serialize};

pub mod external;
mod safe;
pub use safe::*;

/// enum for processor stages
#[derive(Copy, Clone, Debug)]
pub enum Stage {
    /// pre commit phase1
    PC1,

    /// pre commit phase2
    PC2,

    /// commit phase1
    C1,

    /// commit phase2
    C2,
}

impl Stage {
    fn name(&self) -> &'static str {
        match self {
            Stage::PC1 => "pc1",
            Stage::PC2 => "pc2",
            Stage::C1 => "c1",
            Stage::C2 => "c2",
        }
    }
}

impl AsRef<str> for Stage {
    fn as_ref(&self) -> &str {
        self.name()
    }
}

/// type alias of boxed PC2Processor
pub type BoxedPC2Processor = Box<dyn PC2Processor>;

/// type alias of boxed C2Processor
pub type BoxedC2Processor = Box<dyn C2Processor>;

/// abstraction for pre commit phase2 processor
pub trait PC2Processor: Send + Sync {
    /// execute pc2 task
    fn process(
        &self,
        pc1out: SealPreCommitPhase1Output,
        cache_dir: PathBuf,
        sealed_file: PathBuf,
    ) -> Result<SealPreCommitPhase2Output> {
        PC2Input {
            pc1out,
            cache_dir,
            sealed_file,
        }
        .process()
    }
}

/// abstraction for commit phase2 processor
pub trait C2Processor: Send + Sync {
    /// execute c2 task
    fn process(
        &self,
        c1out: SealCommitPhase1Output,
        prover_id: ProverId,
        sector_id: SectorId,
    ) -> Result<SealCommitPhase2Output> {
        C2Input {
            c1out,
            prover_id,
            sector_id,
        }
        .process()
    }
}

/// abstraction for inputs of one stage
pub trait Input: Serialize + DeserializeOwned + Debug + Send
where
    Self::Out: Serialize + DeserializeOwned + Debug + Send,
{
    /// the stage which this input belongs to
    const STAGE: Stage;

    /// the output type
    type Out;

    /// execute the stage
    fn process(self) -> Result<Self::Out>;
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs of stage pc2
pub struct PC2Input {
    pc1out: SealPreCommitPhase1Output,
    cache_dir: PathBuf,
    sealed_file: PathBuf,
}

impl Input for PC2Input {
    const STAGE: Stage = Stage::PC2;
    type Out = SealPreCommitPhase2Output;

    fn process(self) -> Result<Self::Out> {
        seal_pre_commit_phase2(self.pc1out, self.cache_dir, self.sealed_file)
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs of stage c2
pub struct C2Input {
    c1out: SealCommitPhase1Output,
    prover_id: ProverId,
    sector_id: SectorId,
}

impl Input for C2Input {
    const STAGE: Stage = Stage::C2;
    type Out = SealCommitPhase2Output;

    fn process(self) -> Result<Self::Out> {
        seal_commit_phase2(self.c1out, self.prover_id, self.sector_id)
    }
}

pub mod internal {
    //! internal impls

    use super::{C2Processor, PC2Processor};

    /// processor impl for pc2
    pub struct PC2;
    impl PC2Processor for PC2 {}

    /// proceesor impl for c2
    pub struct C2;
    impl C2Processor for C2 {}
}
