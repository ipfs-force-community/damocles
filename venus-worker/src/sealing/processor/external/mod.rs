//! external implementations of processors

use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use rand::distributions::WeightedIndex;
use rand::prelude::Distribution;
use rand::rngs::OsRng;
use rand::seq::SliceRandom;

use self::sub::{ProcessingGuard, SubProcessor};

use super::{Input, ProcessorTrait};
use crate::sealing::resource::Pool;

pub mod config;
pub mod sub;

pub struct ExtProcessor<I>
where
    I: Input,
{
    limit: Arc<Pool>,
    subs: Vec<SubProcessor<I>>,
}

impl<I> ExtProcessor<I>
where
    I: Input,
{
    pub fn build(cfg: &[config::Ext], limit: Arc<Pool>) -> Result<Self> {
        let subs = sub::start_sub_processors(cfg).with_context(|| format!("start sub process for stage {}", I::STAGE))?;

        let proc = Self { limit, subs };

        Ok(proc)
    }
}

fn try_lock<'a, I: Input>(
    txes: impl Iterator<Item = &'a SubProcessor<I>>,
    limit: &Pool,
) -> Result<Vec<(ProcessingGuard, &'a SubProcessor<I>)>> {
    let mut acquired = Vec::new();
    for tx in txes {
        if let Some(guard) = tx.try_lock(limit)? {
            acquired.push((guard, tx));
        }
    }
    Ok(acquired)
}

impl<I> ProcessorTrait<I> for ExtProcessor<I>
where
    I: Input,
{
    fn process(&self, input: I) -> Result<I::Output> {
        let size = self.subs.len();
        if size == 0 {
            return Err(anyhow!("no available sub processor"));
        }

        let (_processing_guard, sub_processor) = {
            let mut acquired = try_lock(self.subs.iter().filter(|s| !s.limiter.is_full()), &self.limit)?;

            if !acquired.is_empty() {
                let dist = WeightedIndex::new(acquired.iter().map(|(_, sub_process_tx)| sub_process_tx.weight))
                    .context("invalid acquired list")?;

                acquired.swap_remove(dist.sample(&mut OsRng))
            } else {
                let chosen = self
                    .subs
                    .choose_weighted(&mut OsRng, |x| x.weight)
                    .context("no input tx from availables chosen")?;
                (chosen.lock(&self.limit)?, chosen)
            }
        };

        sub_processor.producer.process(input)
    }
}
