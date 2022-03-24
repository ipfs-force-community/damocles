//! processor abstractions & implementations for sealing

use std::fmt::Debug;
use std::path::PathBuf;

use anyhow::Result;
use fil_types::ActorID;
use serde::{de::DeserializeOwned, Deserialize, Serialize};

pub mod external;
mod safe;
pub use safe::*;

mod proof;

/// name str for tree_d
pub const STAGE_NAME_TREED: &str = "tree_d";

/// name str for pc1
pub const STAGE_NAME_PC1: &str = "pc1";

/// name str for pc2
pub const STAGE_NAME_PC2: &str = "pc2";

/// name str for c1
pub const STAGE_NAME_C1: &str = "c1";

/// name str for c2
pub const STAGE_NAME_C2: &str = "c2";

/// name str for snap encode
pub const STAGE_NAME_SNAP_ENCODE: &str = "snap_encode";

/// name str for snap prove
pub const STAGE_NAME_SNAP_PROVE: &str = "snap_prove";

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

    /// snap_encode_into
    SnapEncode,

    /// snap_generate_sector_update_proof
    SnapProve,
}

impl Stage {
    fn name(&self) -> &'static str {
        match self {
            Stage::TreeD => STAGE_NAME_TREED,
            Stage::PC1 => STAGE_NAME_PC1,
            Stage::PC2 => STAGE_NAME_PC2,
            Stage::C1 => STAGE_NAME_C1,
            Stage::C2 => STAGE_NAME_C2,
            Stage::SnapEncode => STAGE_NAME_SNAP_ENCODE,
            Stage::SnapProve => STAGE_NAME_SNAP_PROVE,
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
pub type BoxedPC1Processor = BoxedProcessor<PC1Input>;
pub type BoxedPC2Processor = BoxedProcessor<PC2Input>;
pub type BoxedC2Processor = BoxedProcessor<C2Input>;
pub type BoxedSnapEncodeProcessor = BoxedProcessor<SnapEncodeInput>;
pub type BoxedSnapProveProcessor = BoxedProcessor<SnapProveInput>;

pub trait Processor<I>: Send + Sync
where
    I: Input,
{
    fn process(&self, input: I) -> Result<I::Out> {
        input.process()
    }
}

/// abstraction for inputs of one stage
pub trait Input: Serialize + DeserializeOwned + Debug + Send + Sync + 'static
where
    Self::Out: Serialize + DeserializeOwned + Debug + Send + Sync + 'static,
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
    type Out = bool;

    fn process(self) -> Result<Self::Out> {
        create_tree_d(
            self.registered_proof,
            Some(self.staged_file),
            self.cache_dir,
        )
        .map(|_| true)
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PC1Input {
    pub registered_proof: RegisteredSealProof,
    pub cache_path: PathBuf,
    pub in_path: PathBuf,
    pub out_path: PathBuf,
    pub prover_id: ProverId,
    pub sector_id: SectorId,
    pub ticket: Ticket,
    pub piece_infos: Vec<PieceInfo>,
}

impl Input for PC1Input {
    const STAGE: Stage = Stage::PC1;
    type Out = SealPreCommitPhase1Output;

    fn process(self) -> Result<Self::Out> {
        seal_pre_commit_phase1(
            self.registered_proof,
            self.cache_path,
            self.in_path,
            self.out_path,
            self.prover_id,
            self.sector_id,
            self.ticket,
            &self.piece_infos[..],
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
    pub miner_id: ActorID,
}

impl Input for C2Input {
    const STAGE: Stage = Stage::C2;
    type Out = SealCommitPhase2Output;

    fn process(self) -> Result<Self::Out> {
        seal_commit_phase2(self.c1out, self.prover_id, self.sector_id)
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs for stage SnapEncode
pub struct SnapEncodeInput {
    /// field used in snap encode stage
    pub registered_proof: RegisteredUpdateProof,

    /// field used in snap encode stage
    pub new_replica_path: PathBuf,

    /// field used in snap encode stage
    pub new_cache_path: PathBuf,

    /// field used in snap encode stage
    pub sector_path: PathBuf,

    /// field used in snap encode stage
    pub sector_cache_path: PathBuf,

    /// field used in snap encode stage
    pub staged_data_path: PathBuf,

    /// field used in snap encode stage
    pub piece_infos: Vec<PieceInfo>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SnapEncodeOutput {
    pub comm_r_new: Commitment,
    pub comm_r_last_new: Commitment,
    pub comm_d_new: Commitment,
}

impl Input for SnapEncodeInput {
    const STAGE: Stage = Stage::SnapEncode;
    type Out = SnapEncodeOutput;

    fn process(self) -> Result<Self::Out> {
        snap_encode_into(
            self.registered_proof,
            self.new_replica_path,
            self.new_cache_path,
            self.sector_path,
            self.sector_cache_path,
            self.staged_data_path,
            &self.piece_infos[..],
        )
        .map(|out| SnapEncodeOutput {
            comm_r_new: out.comm_r_new,
            comm_r_last_new: out.comm_r_last_new,
            comm_d_new: out.comm_d_new,
        })
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs for stage SnapProve
pub struct SnapProveInput {
    /// field of registered update proof type
    pub registered_proof: RegisteredUpdateProof,

    /// field of vannilla proofs
    pub vannilla_proofs: Vec<Vec<u8>>,

    /// field of old comm_r
    pub comm_r_old: Commitment,

    /// field of new comm_r
    pub comm_r_new: Commitment,

    /// field of new comm_d
    pub comm_d_new: Commitment,
}

pub type SnapProveOutput = Vec<u8>;

impl Input for SnapProveInput {
    const STAGE: Stage = Stage::SnapProve;
    type Out = SnapProveOutput;

    fn process(self) -> Result<Self::Out> {
        snap_generate_sector_update_proof(
            self.registered_proof,
            self.vannilla_proofs
                .into_iter()
                .map(|p| PartitionProofBytes(p))
                .collect(),
            self.comm_r_old,
            self.comm_r_new,
            self.comm_d_new,
        )
        .map(|out| out.0)
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
