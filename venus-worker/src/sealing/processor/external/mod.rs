//! external implementations of processors

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, Sender};
use rand::rngs::OsRng;
use rand::seq::SliceRandom;

use super::*;

pub mod config;
pub mod sub;

pub struct ExtProcessor<I>
where
    I: Input,
{
    txes: Vec<(Sender<(I, Sender<Result<I::Out>>)>, Sender<()>)>,
}

impl<I> ExtProcessor<I>
where
    I: Input,
{
    pub fn build(cfg: &Vec<config::Ext>) -> Result<(Self, Vec<sub::SubProcess<I>>)> {
        let (txes, subproc) = sub::start_sub_processes(cfg)
            .with_context(|| format!("start sub process for stage {}", I::STAGE.name()))?;

        let proc = Self { txes };

        Ok((proc, subproc))
    }
}

impl<I> Processor<I> for ExtProcessor<I>
where
    I: Input,
{
    fn process(&self, input: I) -> Result<I::Out> {
        let size = self.txes.len();
        if size == 0 {
            return Err(anyhow!("no available sub processor"));
        }

        let available: Vec<_> = self.txes.iter().filter(|l| !l.1.is_full()).collect();
        let available_size = available.len();

        let input_tx = if available_size == 0 {
            self.txes
                .choose(&mut OsRng)
                .context("no input tx from all chosen")?
        } else {
            &available
                .choose(&mut OsRng)
                .context("no input tx from availables chosen")?
        };

        let (res_tx, res_rx) = bounded(1);
        input_tx.0.send((input, res_tx)).map_err(|e| {
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
