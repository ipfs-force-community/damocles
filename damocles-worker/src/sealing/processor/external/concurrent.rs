use anyhow::Result;
use crossbeam_channel::{bounded, unbounded, Receiver, Sender};

use crate::sealing::processor::LockProcessor;

use super::load::TryLockProcessor;

pub struct Guard<G> {
    inner: G,
    limit_rx: Receiver<()>,
}

impl<G: std::ops::Deref> std::ops::Deref for Guard<G> {
    type Target = <G as std::ops::Deref>::Target;

    fn deref(&self) -> &Self::Target {
        self.inner.deref()
    }
}

impl<G> Drop for Guard<G> {
    fn drop(&mut self) {
        let _ = self.limit_rx.recv();
    }
}

pub struct Concurrent<P> {
    inner: P,
    limit_tx: Sender<()>,
    limit_rx: Receiver<()>,
}

impl<P> Concurrent<P> {
    pub fn new(inner: P, concurrent: Option<usize>) -> Self {
        let (limit_tx, limit_rx) = match concurrent {
            Some(n) => bounded(n),
            None => unbounded(),
        };
        Self { inner, limit_tx, limit_rx }
    }
}

impl<P: LockProcessor> LockProcessor for Concurrent<P> {
    type Guard<'a> = Guard<P::Guard<'a>> where P: 'a;

    fn lock(&self) -> Self::Guard<'_> {
        let inner = self.inner.lock();
        self.limit_tx.send(()).expect("limit channel never disconnect");
        Guard {
            inner,
            limit_rx: self.limit_rx.clone(),
        }
    }
}

impl<P: TryLockProcessor> TryLockProcessor for Concurrent<P> {
    fn try_lock(&self) -> Option<Self::Guard<'_>> {
        let inner = match self.inner.try_lock() {
            Some(inner) => inner,
            None => return None,
        };
        self.limit_tx.try_send(()).ok().map(|_| Guard {
            inner,
            limit_rx: self.limit_rx.clone(),
        })
    }
}
