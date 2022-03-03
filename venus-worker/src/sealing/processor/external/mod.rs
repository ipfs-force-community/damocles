//! external implementations of processors

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, Sender};

use super::*;

pub mod config;
pub mod sub;

pub struct ExtProcessor<I>
where
    I: Input,
{
    input_tx: Sender<(I, Sender<Result<I::Out>>)>,
}

impl<I> ExtProcessor<I>
where
    I: Input,
{
    pub fn build(cfg: &Vec<config::Ext>) -> Result<(Self, Vec<sub::SubProcess<I>>)> {
        let (input_tx, input_rx) = bounded(0);
        let subproc = sub::start_sub_processes(cfg, input_rx)
            .with_context(|| format!("start sub process for stage {}", I::STAGE.name()))?;

        let proc = Self { input_tx };

        Ok((proc, subproc))
    }
}

impl<I> Processor<I> for ExtProcessor<I>
where
    I: Input,
{
    fn process(&self, input: I) -> Result<I::Out> {
        let (res_tx, res_rx) = bounded(0);
        self.input_tx.send((input, res_tx)).map_err(|e| {
            anyhow!(
                "failed to send input through chan for stage {}: {:?}",
                I::STAGE.name(),
                e
            )
        })?;

        let res = res_rx
            .recv()
            .with_context(|| format!("recv process result for stage {}", I::STAGE.name()))?;

        res
    }
}
