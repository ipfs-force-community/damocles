//! external implementations of processors

use anyhow::Result;
use crossbeam_channel::{bounded, Sender};

use super::*;

pub mod config;
pub mod sub;

type TreeDInputSender = Sender<(TreeDInput, Sender<Result<()>>)>;

type PC2InputSender = Sender<(PC2Input, Sender<Result<SealPreCommitPhase2Output>>)>;

type C2InputSender = Sender<(C2Input, Sender<Result<SealCommitPhase2Output>>)>;

/// processor impl for pc2
pub struct TreeD {
    input_tx: TreeDInputSender,
}

impl TreeD {
    /// build a PC2 instance
    pub fn build(cfg: &config::Ext) -> Result<(Self, Vec<sub::SubProcess<TreeDInput>>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_processes(cfg, input_rx)?;

        let tree_d = TreeD { input_tx };

        Ok((tree_d, subproc))
    }
}

impl TreeDProcessor for TreeD {
    fn process(
        &self,
        registered_proof: RegisteredSealProof,
        staged_file: PathBuf,
        cache_dir: PathBuf,
    ) -> Result<()> {
        let (res_tx, res_rx) = bounded(0);
        self.input_tx.send((
            TreeDInput {
                registered_proof,
                staged_file,
                cache_dir,
            },
            res_tx,
        ))?;

        let res = res_rx.recv()?;
        res
    }
}

/// processor impl for pc2
pub struct PC2 {
    input_tx: PC2InputSender,
}

impl PC2 {
    /// build a PC2 instance
    pub fn build(cfg: &config::Ext) -> Result<(Self, Vec<sub::SubProcess<PC2Input>>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_processes(cfg, input_rx)?;

        let pc2 = PC2 { input_tx };

        Ok((pc2, subproc))
    }
}

impl PC2Processor for PC2 {
    fn process(
        &self,
        pc1out: SealPreCommitPhase1Output,
        cache_dir: PathBuf,
        sealed_file: PathBuf,
    ) -> Result<SealPreCommitPhase2Output> {
        let (res_tx, res_rx) = bounded(0);
        self.input_tx.send((
            PC2Input {
                pc1out,
                cache_dir,
                sealed_file,
            },
            res_tx,
        ))?;

        let res = res_rx.recv()?;
        res
    }
}

/// processor impl for c2
pub struct C2 {
    input_tx: C2InputSender,
}

impl C2 {
    /// build a C2 instance
    pub fn build(cfg: &config::Ext) -> Result<(Self, Vec<sub::SubProcess<C2Input>>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_processes(cfg, input_rx)?;

        let c2 = C2 { input_tx };

        Ok((c2, subproc))
    }
}

impl C2Processor for C2 {
    fn process(
        &self,
        c1out: SealCommitPhase1Output,
        prover_id: ProverId,
        sector_id: SectorId,
    ) -> Result<SealCommitPhase2Output> {
        let (res_tx, res_rx) = bounded(0);
        self.input_tx.send((
            C2Input {
                c1out,
                prover_id,
                sector_id,
            },
            res_tx,
        ))?;

        let res = res_rx.recv()?;
        res
    }
}
