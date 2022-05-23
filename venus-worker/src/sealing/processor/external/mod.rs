//! external implementations of processors

use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::bounded;
use rand::distributions::WeightedIndex;
use rand::prelude::Distribution;
use rand::rngs::OsRng;
use rand::seq::SliceRandom;

use self::sub::{ProcessingGuard, SubProcessTx};

use super::*;
use crate::sealing::resource::Pool;

pub mod config;
pub mod sub;

pub struct ExtProcessor<I>
where
    I: Input,
{
    limit: Arc<Pool>,
    txes: Vec<sub::SubProcessTx<I>>,
}

impl<I> ExtProcessor<I>
where
    I: Input,
{
    pub fn build(cfg: &[config::Ext], limit: Arc<Pool>) -> Result<(Self, Vec<sub::SubProcess<I>>)> {
        let sub::SubProcessContext { txes, processes: subproc } =
            sub::start_sub_processes(cfg).with_context(|| format!("start sub process for stage {}", I::STAGE.name()))?;

        let proc = Self { limit, txes };

        Ok((proc, subproc))
    }
}

fn try_lock<'a, I: Input>(txes: &Vec<&'a SubProcessTx<I>>, limit: &Pool) -> Result<Vec<(ProcessingGuard, &'a SubProcessTx<I>)>> {
    let mut acquired = Vec::new();
    for &tx in txes {
        if let Some(guard) = tx.try_lock(limit)? {
            acquired.push((guard, tx));
        }
    }
    Ok(acquired)
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
        let available: Vec<_> = self.txes.iter().filter(|s| !s.limiter.is_full()).collect();

        let (_processing_guard, sub_process_tx) = {
            let mut acquired = try_lock(&available, &self.limit)?;

            if !acquired.is_empty() {
                let dist = WeightedIndex::new(acquired.iter().map(|(_, sub_process_tx)| sub_process_tx.weight))
                    .context("invalid acquired list")?;

                acquired.swap_remove(dist.sample(&mut OsRng))
            } else {
                let chosen = available
                    .choose_weighted(&mut OsRng, |x| x.weight)
                    .context("no input tx from availables chosen")?;
                (chosen.lock(&self.limit)?, *chosen)
            }
        };

        let (res_tx, res_rx) = bounded(1);
        sub_process_tx
            .input_tx
            .send((input, res_tx))
            .map_err(|e| anyhow!("failed to send input through chan for stage {}: {:?}", I::STAGE.name(), e))?;

        let res = res_rx
            .recv()
            .with_context(|| format!("recv process result for stage {}", I::STAGE.name()))?;

        res
    }
}
