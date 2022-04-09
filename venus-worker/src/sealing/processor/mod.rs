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
    SnapReplicaUpdate,

    /// snap_generate_sector_update_proof
    SnapProveReplicaUpdate,
}

impl Stage {
    fn name(&self) -> &'static str {
        match self {
            Stage::TreeD => "tree_d",
            Stage::PC1 => "pc1",
            Stage::PC2 => "pc2",
            Stage::C1 => "c1",
            Stage::C2 => "c2",
            Stage::SnapReplicaUpdate => "snap-ru",
            Stage::SnapProveReplicaUpdate => "snap-pru",
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
/// inputs for stage SnapReplicaUpdate
pub struct SnapReplicaUpdateInput {
    registered_proof: RegisteredUpdateProof,
    new_replica_path: PathBuf,
    new_cache_path: PathBuf,
    sector_path: PathBuf,
    sector_cache_path: PathBuf,
    staged_data_path: PathBuf,
    piece_infos: Vec<PieceInfo>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SnapReplicaUpdateOutput {
    pub comm_r_new: Commitment,
    pub comm_r_last_new: Commitment,
    pub comm_d_new: Commitment,
}

impl Input for SnapReplicaUpdateInput {
    const STAGE: Stage = Stage::SnapReplicaUpdate;
    type Out = SnapReplicaUpdateOutput;

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
        .map(|out| SnapReplicaUpdateOutput {
            comm_r_new: out.comm_r_new,
            comm_r_last_new: out.comm_r_last_new,
            comm_d_new: out.comm_d_new,
        })
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
/// inputs for stage SnapProveReplicaUpdate
pub struct SnapProveReplicaUpdateInput {
    registered_proof: RegisteredUpdateProof,
    vannilla_proofs: Vec<Vec<u8>>,
    comm_r_old: Commitment,
    comm_r_new: Commitment,
    comm_d_new: Commitment,
}

impl Input for SnapProveReplicaUpdateInput {
    const STAGE: Stage = Stage::SnapProveReplicaUpdate;
    type Out = Vec<u8>;

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
