use anyhow::Result;

use crate::sealing::resource::{LimitItem, Pool, Token};

pub struct SealingLimitBuilder {
    pool: Pool,
}

impl SealingLimitBuilder {
    pub fn new() -> Self {
        Self {
            pool: Pool::empty(),
        }
    }

    pub fn build(self) -> SealingLimit {
        SealingLimit { pool: self.pool }
    }

    pub fn extend_ext_locks_limit<I, K>(&mut self, limits: I) -> &mut Self
    where
        I: IntoIterator<Item = (K, usize)>,
        K: AsRef<str>,
    {
        self.pool.extend(limits.into_iter().map(|(limit_key, num)| {
            LimitItem {
                name: ext_lock_limit_key(limit_key),
                concurrent: Some(num),
                staggered_interval: None,
            }
        }));
        self
    }

    pub fn extend_stage_limits<I>(&mut self, limits: I) -> &mut Self
    where
        I: IntoIterator<Item = LimitItem>,
    {
        self.pool.extend(limits.into_iter().map(|item| LimitItem {
            name: stage_limit_key(item.name),
            concurrent: item.concurrent,
            staggered_interval: item.staggered_interval,
        }));
        self
    }
}

pub struct SealingLimit {
    pool: Pool,
}

impl SealingLimit {
    pub fn acquire_ext_lock<K: AsRef<str>>(&self, name: K) -> Result<Token> {
        self.pool.acquire(ext_lock_limit_key(name))
    }

    pub fn try_acquire_ext_lock<K: AsRef<str>>(
        &self,
        name: K,
    ) -> Result<Option<Token>> {
        self.pool.try_acquire(ext_lock_limit_key(name))
    }

    pub fn acquire_stage_limit<K: AsRef<str>>(&self, name: K) -> Result<Token> {
        self.pool.acquire(stage_limit_key(name))
    }

    #[allow(dead_code)]
    pub fn try_acquire_stage_limit<K: AsRef<str>>(
        &self,
        name: K,
    ) -> Result<Option<Token>> {
        self.pool.try_acquire(stage_limit_key(name))
    }
}

fn ext_lock_limit_key(k: impl AsRef<str>) -> String {
    format!("ext-{}", k.as_ref())
}

fn stage_limit_key(k: impl AsRef<str>) -> String {
    format!("stage-{}", k.as_ref())
}
