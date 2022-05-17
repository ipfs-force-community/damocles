//! resource control, acquiring & releasing

use std::{collections::HashMap, time::Duration};

use anyhow::Result;
use crossbeam_channel::{bounded, Receiver, Sender};
use leaky_bucket::RateLimiter;

use crate::{block_on, logging::debug};

const LOG_TARGET: &str = "resource";

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
                .refill(1)
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
            let staggered_limit_opt = limit_item
                .staggered_interval
                .map(|interval| StaggeredLimit::new(1, interval.to_owned()));
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

#[cfg(test)]
mod tests {
    use humantime::format_duration;
    use tokio::runtime::Builder;

    use super::{LimitItem, Pool};
    use std::time::{Duration, Instant};

    macro_rules! assert_elapsed {
        ($start:expr, $dur:expr) => {{
            assert_elapsed!($start, $dur, "");
        }};
        ($start:expr, $dur:expr, $($arg:tt)+) => {{
            let elapsed = $start.elapsed();
            // type ascription improves compiler error when wrong type is passed
            let lower: std::time::Duration = $dur;

            // Handles ms rounding
            assert!(
                elapsed >= lower && elapsed <= lower + std::time::Duration::from_millis(2),
                "actual = {:?}, expected = {:?}. {}",
                elapsed,
                lower,
                format_args!($($arg)+),
            );
        }};
    }

    #[test]
    fn test_acquire_both_concurrent_limit_and_staggered_limit() {
        let rt = Builder::new_multi_thread().enable_all().build().unwrap();
        let _rt_guard = rt.enter();

        let staggered_interval = ms(10);
        let pool = Pool::new(
            (vec![LimitItem {
                name: "pc1",
                concurrent: Some(&2),
                staggered_interval: Some(&staggered_interval),
            }])
            .into_iter(),
        );
        let mut now = Instant::now();
        let token1 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "first acquire should not block");
        now = Instant::now();
        let _token2 = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval,
            "because of the staggered limit, it should to block for {}",
            format_duration(staggered_interval).to_string()
        );

        now = Instant::now();
        timeout(ms(20), move || drop(token1));
        let _ = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(20), "concurrent is only 2 so must wait for for the `token1` to be dropped");

        drop(_token2);

        now = Instant::now();
        let _ = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval,
            "need wait staggered_interval: {}",
            format_duration(staggered_interval).to_string()
        );
    }

    #[test]
    fn test_acquire_only_concurrent_limit() {
        let rt = Builder::new_multi_thread().enable_all().build().unwrap();
        let _rt_guard = rt.enter();

        let pool = Pool::new(
            (vec![LimitItem {
                name: "pc1",
                concurrent: Some(&2),
                staggered_interval: None,
            }])
            .into_iter(),
        );

        let mut now = Instant::now();
        let token1 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "first acquire should not block");
        assert!(token1.is_some());

        now = Instant::now();
        let token2 = pool.acquire("pc1").unwrap();
        assert!(token2.is_some());
        assert_elapsed!(now, ms(0), "seconed acquire should not block");

        now = Instant::now();
        timeout(ms(20), move || drop(token1));
        assert!(pool.acquire("pc1").unwrap().is_some());
        assert_elapsed!(now, ms(20), "concurrent is only 2 so must wait for for the `token1` to be dropped");

        drop(token2);

        now = Instant::now();
        assert!(pool.acquire("pc1").unwrap().is_some());
        assert_elapsed!(now, ms(0), "should not block");
    }

    #[test]
    fn test_acquire_only_staggered_limit() {
        let rt = Builder::new_multi_thread().enable_all().build().unwrap();
        let _rt_guard = rt.enter();

        let staggered_interval = ms(10);
        let pool = Pool::new(
            (vec![LimitItem {
                name: "pc1",
                concurrent: None,
                staggered_interval: Some(&staggered_interval),
            }])
            .into_iter(),
        );

        let mut now = Instant::now();
        assert!(pool.acquire("pc1").unwrap().is_none());
        assert_elapsed!(now, ms(0), "first acquire should not block");

        now = Instant::now();
        assert!(pool.acquire("pc1").unwrap().is_none());
        assert_elapsed!(
            now,
            staggered_interval,
            "because of the staggered limit, it should to block for {}",
            format_duration(staggered_interval).to_string()
        );

        now = Instant::now();
        let _ = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval,
            "need wait staggered_interval: {}",
            format_duration(staggered_interval).to_string()
        );
    }

    fn ms(n: u64) -> Duration {
        Duration::from_millis(n)
    }

    fn timeout(t: Duration, f: impl FnOnce() -> () + Send + 'static) {
        let _ = std::thread::spawn(move || {
            std::thread::sleep(t);
            f();
        });
    }
}
