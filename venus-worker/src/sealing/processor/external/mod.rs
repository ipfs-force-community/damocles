//! external implementations of processors

use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, Sender};
use rand::rngs::OsRng;
use rand::seq::SliceRandom;

use super::*;
use crate::logging::debug;
use crate::sealing::resource::Pool;

pub mod config;
pub mod sub;

pub struct ExtProcessor<I>
where
    I: Input,
{
    limit: Arc<Pool>,
    txes: Vec<(Sender<(I, Sender<Result<I::Out>>)>, Sender<()>, Vec<String>)>,
}

impl<I> ExtProcessor<I>
where
    I: Input,
{
    pub fn build(
        cfg: &Vec<config::Ext>,
        limit: Arc<Pool>,
    ) -> Result<(Self, Vec<sub::SubProcess<I>>)> {
        let (txes, subproc) = sub::start_sub_processes(cfg)
            .with_context(|| format!("start sub process for stage {}", I::STAGE.name()))?;

        let proc = Self { limit, txes };

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

        let tokens = input_tx
            .2
            .iter()
            .map(|lock_name| {
                let token = self
                    .limit
                    .acquire(lock_name)
                    .with_context(|| format!("acquire lock for {}", lock_name))?;

                debug!(
                    name = lock_name.as_str(),
                    stage = I::STAGE.name(),
                    got = token.is_some(),
                    "acquiring lock",
                );

                Ok(token)
            })
            .collect::<Result<Vec<_>>>()?;

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

        tokens.into_iter().for_each(drop);

        res
    }
}
