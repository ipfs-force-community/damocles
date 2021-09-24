//! processor abstractions & implementations for sealing

use std::fmt::Debug;
use std::path::PathBuf;

use anyhow::Result;
use serde::{de::DeserializeOwned, Deserialize, Serialize};

pub mod external;
mod safe;
pub use safe::*;
mod proof;

/// enum for processor stages
#[derive(Copy, Clone, Debug)]
pub enum Stage {
    /// construct tree d
    TreeD,

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
            Stage::TreeD => "tree_d",
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

pub type BoxedProcessor<I> = Box<dyn Processor<I>>;
pub type BoxedTreeDProcessor = BoxedProcessor<TreeDInput>;
pub type BoxedPC2Processor = BoxedProcessor<PC2Input>;
pub type BoxedC2Processor = BoxedProcessor<C2Input>;

pub trait Processor<I>: Send + Sync
where
    I: Input,
{
    fn process(&self, input: I) -> Result<I::Out> {
        input.process()
    }
}

/// abstraction for inputs of one stage
pub trait Input: Serialize + DeserializeOwned + Debug + Send + Sync
where
    Self::Out: Serialize + DeserializeOwned + Debug + Send + Sync,
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
pub struct TreeDInput {
    pub registered_proof: RegisteredSealProof,
    pub staged_file: PathBuf,
    pub cache_dir: PathBuf,
}

impl Input for TreeDInput {
    const STAGE: Stage = Stage::TreeD;
    type Out = ();

    fn process(self) -> Result<Self::Out> {
        create_tree_d(
            self.registered_proof,
            Some(self.staged_file),
            self.cache_dir,
        )
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs of stage pc2
pub struct PC2Input {
    pub pc1out: SealPreCommitPhase1Output,
    pub cache_dir: PathBuf,
    pub sealed_file: PathBuf,
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
    pub c1out: SealCommitPhase1Output,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
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

    use std::marker::PhantomData;

    use super::{Input, Processor};

    pub struct Proc<I: Input> {
        _data: PhantomData<I>,
    }

    impl<I> Proc<I>
    where
        I: Input,
    {
        pub fn new() -> Self {
            Proc {
                _data: Default::default(),
            }
        }
    }

    impl<I> Processor<I> for Proc<I> where I: Input {}
}
