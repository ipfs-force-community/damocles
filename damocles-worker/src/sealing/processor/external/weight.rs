use crate::sealing::processor::LockProcessor;
use anyhow::{Context, Result};
use rand::{
    distributions::{Distribution, WeightedIndex},
    rngs::OsRng,
    seq::SliceRandom,
};

pub trait Weighted {
    fn weight(&self) -> u16;
}

pub trait TryLockProcessor: LockProcessor {
    fn try_lock(&self) -> Result<Option<Self::Guard<'_>>>;
}

pub struct Weight<P> {
    inner: Vec<P>,
}

impl<P> Weight<P> {
    pub fn new(inner: Vec<P>) -> Self {
        Self { inner }
    }
}

impl<P> LockProcessor for Weight<P>
where
    P: LockProcessor + TryLockProcessor + Weighted,
{
    type Guard<'a> = P::Guard<'a> where P: 'a;

    fn lock(&self) -> Result<Self::Guard<'_>> {
        let mut acquired = Vec::new();
        for p in &self.inner {
            if let Some(guard) = p.try_lock()? {
                acquired.push((guard, p.weight()));
            }
        }
        Ok(match acquired.len() {
            0 => {
                let chosen = self
                    .inner
                    .choose_weighted(&mut OsRng, |x| x.weight())
                    .context("no available processors")?;
                chosen.lock()?
            }
            1 => acquired.pop().unwrap().0,
            _ => {
                let target = WeightedIndex::new(acquired.iter().map(|(_, weight)| weight)).context("no available processors")?;
                acquired.swap_remove(target.sample(&mut OsRng)).0
            }
        })
    }
}
