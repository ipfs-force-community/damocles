//! external implementations of processors

use anyhow::Result;
use crossbeam_channel::{bounded, Receiver, Sender};

use super::*;

pub mod config;
pub mod sub;

type PC2InputSender = Sender<(PC2Input, Sender<Result<SealPreCommitPhase2Output>>)>;

type C2InputSender = Sender<(C2Input, Sender<Result<SealCommitPhase2Output>>)>;

/// processor impl for pc2
pub struct PC2 {
    input_tx: PC2InputSender,

    res_tx: Sender<Result<SealPreCommitPhase2Output>>,
    res_rx: Receiver<Result<SealPreCommitPhase2Output>>,
}

impl PC2 {
    /// build a PC2 instance
    pub fn build(cfg: &config::Ext) -> Result<(Self, sub::SubProcess<PC2Input>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_process(cfg, input_rx)?;
        let (res_tx, res_rx) = bounded(0);

        let pc2 = PC2 {
            input_tx,
            res_tx,
            res_rx,
        };

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
        self.input_tx.send((
            PC2Input {
                pc1out,
                cache_dir,
                sealed_file,
            },
            self.res_tx.clone(),
        ))?;

        let res = self.res_rx.recv()?;
        res
    }
}

/// processor impl for c2
pub struct C2 {
    input_tx: C2InputSender,

    res_tx: Sender<Result<SealCommitPhase2Output>>,
    res_rx: Receiver<Result<SealCommitPhase2Output>>,
}

impl C2 {
    /// build a C2 instance
    pub fn build(cfg: &config::Ext) -> Result<(Self, sub::SubProcess<C2Input>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_process(cfg, input_rx)?;
        let (res_tx, res_rx) = bounded(0);

        let c2 = C2 {
            input_tx,
            res_tx,
            res_rx,
        };

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
        self.input_tx.send((
            C2Input {
                c1out,
                prover_id,
                sector_id,
            },
            self.res_tx.clone(),
        ))?;

        let res = self.res_rx.recv()?;
        res
    }
}
