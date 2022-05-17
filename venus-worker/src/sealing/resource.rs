//! resource control, acquiring & releasing

use std::{collections::HashMap, time::Duration};

use anyhow::Result;
use crossbeam_channel::{bounded, Receiver, Sender};
use leaky_bucket::RateLimiter;

use crate::{block_on, logging::debug};

const LOG_TARGET: &str = "resource";

/// The proportion of tasks allowed to start
/// at the same time to the total concurrency of tasks
const STAGGERED_CONCURRENT_PROPORTION: f32 = 0.1;

type LimitTx = Sender<()>;
type LimitRx = Receiver<()>;

/// Token represents 1 available resource unit
pub struct Token(LimitRx);

impl Drop for Token {
    fn drop(&mut self) {
        let _ = self.0.recv();
    }
}

struct ConcurrentLimit {
    tx: LimitTx,
    rx: LimitRx,
}

impl ConcurrentLimit {
    fn new(size: usize) -> Self {
        let (tx, rx) = bounded(size);

        ConcurrentLimit { tx, rx }
    }

    fn acquire(&self) -> Result<Token> {
        self.tx.send(())?;

        Ok(Token(self.rx.clone()))
    }
}

/// StaggeredLimit is used to avoid the multiple concurrent tasks
/// that start at the same time, causing excessive strain
/// on resources such as cpu or disk.
/// StaggeredLimit uses an [leaky bucket] algorithm to allow
/// multiple tasks to start at staggered times.
struct StaggeredLimit(RateLimiter);

impl StaggeredLimit {
    /// Create a StaggeredLimit with specified concurrent and specified interval.
    ///
    /// * `concurrent` - The number of tasks that are allowed to start at the same time
    /// * `interval` - Time interval for task start
    fn new(concurrent: usize, interval: Duration) -> Self {
        Self(
            RateLimiter::builder()
                .max(concurrent)
                .initial(concurrent)
                .interval(interval)
                .build(),
        )
    }

    fn wait(&self) {
        block_on(self.0.acquire_one())
    }
}

/// resource limit pool
pub struct Pool {
    pool: HashMap<String, (Option<ConcurrentLimit>, Option<StaggeredLimit>)>,
}

#[derive(Debug)]
pub struct LimitItem<'a> {
    pub name: &'a str,
    pub concurrent: Option<&'a usize>,
    pub staggered_interval: Option<&'a Duration>,
}

impl Pool {
    /// construct a pool with given name-size mapping
    pub fn new<'a, I: Iterator<Item = LimitItem<'a>>>(iter: I) -> Self {
        let mut pool = HashMap::new();

        for limit_item in iter {
            debug!(
                target: LOG_TARGET,
                name = limit_item.name,
                concurrent = limit_item.concurrent,
                staggered_time_interval = limit_item.staggered_interval.cloned().unwrap_or_default().as_secs_f32(),
                "add limitation"
            );

            let concurrent_limit_opt = limit_item.concurrent.map(|concurrent| ConcurrentLimit::new(*concurrent));
            let staggered_limit_opt = limit_item.staggered_interval.map(|interval| {
                let c = *limit_item.concurrent.unwrap_or(&0) as f32 * STAGGERED_CONCURRENT_PROPORTION;
                StaggeredLimit::new(
                    1.max(c as usize), // ensure the minimum concurrency is 1
                    interval.to_owned(),
                )
            });
            pool.insert(limit_item.name.to_string(), (concurrent_limit_opt, staggered_limit_opt));
        }

        Pool { pool }
    }

    /// acquires a token for the named resource
    pub fn acquire<N: AsRef<str>>(&self, name: N) -> Result<Option<Token>> {
        let key = name.as_ref();
        Ok(match self.pool.get(key) {
            None | Some((None, None)) => {
                debug!(target: LOG_TARGET, name = key, "unlimited");
                None
            }
            Some((concurrent_limit_opt, staggered_limit_opt)) => {
                let mut token_opt = None;
                if let Some(concurrent_limit) = concurrent_limit_opt {
                    token_opt = Some(concurrent_limit.acquire()?)
                }

                if let Some(staggered_limit) = staggered_limit_opt {
                    staggered_limit.wait();
                }
                debug!(target: LOG_TARGET, name = key, "acquired");
                token_opt
            }
        })
    }
}
