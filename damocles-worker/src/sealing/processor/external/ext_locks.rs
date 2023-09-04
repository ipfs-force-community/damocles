use std::sync::Arc;

use anyhow::Result;

use crate::{
    limit::SealingLimit,
    sealing::{processor::LockProcessor, resource},
};

use super::load::TryLockProcessor;

pub struct Guard<G> {
    inner: G,
    _tokens: Vec<resource::Token>,
}

impl<G: std::ops::Deref> std::ops::Deref for Guard<G> {
    type Target = <G as std::ops::Deref>::Target;

    fn deref(&self) -> &Self::Target {
        self.inner.deref()
    }
}

pub struct ExtLocks<P> {
    inner: P,
    locks: Vec<String>,
    limit: Arc<SealingLimit>,
}

impl<P> ExtLocks<P> {
    pub fn new(inner: P, locks: Vec<String>, limit: Arc<SealingLimit>) -> Self {
        Self {
            inner,
            locks,
            limit,
        }
    }
}

impl<P: LockProcessor> LockProcessor for ExtLocks<P> {
    type Guard<'a> = Guard<P::Guard<'a>> where P: 'a;

    fn lock(&self) -> Self::Guard<'_> {
        let inner = self.inner.lock();
        let mut tokens = Vec::new();
        for lock_name in &self.locks {
            tracing::debug!(name = lock_name.as_str(), "acquiring ext lock");
            tokens.push(
                self.limit
                    .acquire_ext_lock(lock_name)
                    .expect("acquire ext lock must ok"),
            )
        }
        Guard {
            inner,
            _tokens: tokens,
        }
    }
}

impl<P: TryLockProcessor> TryLockProcessor for ExtLocks<P> {
    fn try_lock(&self) -> Option<Self::Guard<'_>> {
        let inner = match self.inner.try_lock() {
            Some(inner) => inner,
            None => return None,
        };

        let mut tokens = Vec::new();
        for lock_name in &self.locks {
            tracing::debug!(name = lock_name.as_str(), "acquiring ext lock");
            match self
                .limit
                .try_acquire_ext_lock(lock_name)
                .expect("try acquire ext lock must ok")
            {
                Some(t) => tokens.push(t),
                None => return None,
            }
        }
        Some(Guard {
            inner,
            _tokens: tokens,
        })
    }
}
