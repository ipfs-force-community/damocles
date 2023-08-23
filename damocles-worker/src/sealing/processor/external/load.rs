use std::iter::Enumerate;
use std::sync::{
    atomic::{AtomicU32, Ordering},
    Arc,
};

use anyhow::Result;

use crate::sealing::processor::LockProcessor;

pub trait TryLockProcessor: LockProcessor {
    fn try_lock(&self) -> Option<Self::Guard<'_>>;
}

pub trait Load {
    fn load(&self) -> u32;
    fn max_concurrent(&self) -> u32;
}

pub struct WorkloadSelector<P> {
    inner: Vec<P>,
}

impl<P> WorkloadSelector<P> {
    pub fn new(inner: Vec<P>) -> Self {
        Self { inner }
    }
}

impl<P> LockProcessor for WorkloadSelector<P>
where
    P: LockProcessor + TryLockProcessor + Load,
{
    type Guard<'a> = P::Guard<'a> where P: 'a;

    fn lock(&self) -> Self::Guard<'_> {
        let mut acquired = self
            .inner
            .iter()
            .filter_map(|p| {
                p.try_lock().map(|guard| {
                    let cap = p.max_concurrent().saturating_sub(p.load());
                    (guard, cap)
                })
            })
            .collect::<Vec<_>>();

        if acquired.is_empty() {
            return self.inner.first().expect("no processors").lock();
        }

        tracing::trace!("acquired: {:?}", acquired.iter().map(|(_, cap)| *cap).collect::<Vec<_>>());

        let (mut selected, mut max_cap) = acquired.pop().unwrap();

        while let Some((guard, cap)) = acquired.pop() {
            if cap > max_cap {
                selected = guard;
                max_cap = cap;
            }
        }
        tracing::trace!("selected: {}", max_cap);

        selected
    }
}

pub struct Workload<P> {
    load: Arc<AtomicU32>,
    max_concurrent: u32,
    inner: P,
}

impl<P> Workload<P> {
    const DEFAULT_CONCURRENT: u32 = 64;

    pub fn new(inner: P, max_concurrent: Option<u32>) -> Self {
        Self {
            load: Arc::new(AtomicU32::new(0)),
            inner,
            max_concurrent: max_concurrent.unwrap_or(Self::DEFAULT_CONCURRENT),
        }
    }
}

impl<P> Load for Workload<P> {
    fn load(&self) -> u32 {
        self.load.load(Ordering::SeqCst)
    }

    fn max_concurrent(&self) -> u32 {
        self.max_concurrent
    }
}

impl<P: LockProcessor> LockProcessor for Workload<P> {
    type Guard<'a> = Guard<P::Guard<'a>> where P: 'a;

    fn lock(&self) -> Self::Guard<'_> {
        Guard::new(self.inner.lock(), self.load.clone())
    }
}

impl<P: TryLockProcessor> TryLockProcessor for Workload<P> {
    fn try_lock(&self) -> Option<Self::Guard<'_>> {
        self.inner.try_lock().map(|inner| Guard::new(inner, self.load.clone()))
    }
}

pub struct Guard<G> {
    inner: G,
    load: Arc<AtomicU32>,
}

impl<G> Guard<G> {
    fn new(inner: G, load: Arc<AtomicU32>) -> Self {
        load.fetch_add(1, Ordering::SeqCst);
        Self { inner, load }
    }
}

impl<G: std::ops::Deref> std::ops::Deref for Guard<G> {
    type Target = <G as std::ops::Deref>::Target;

    fn deref(&self) -> &Self::Target {
        self.inner.deref()
    }
}

impl<G> Drop for Guard<G> {
    fn drop(&mut self) {
        self.load.fetch_sub(1, Ordering::SeqCst);
    }
}
