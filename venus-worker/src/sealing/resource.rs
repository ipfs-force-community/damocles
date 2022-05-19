//! resource control, acquiring & releasing

use std::{collections::HashMap, time::Duration};

use anyhow::Result;
use crossbeam_channel::{bounded, select, tick, Receiver, Sender, TryRecvError};

use crate::logging::{debug, warn};

const LOG_TARGET: &str = "resource";

type LimitTx = Sender<()>;
type LimitRx = Receiver<()>;

/// ConcurrentToken represents 1 available resource unit
pub struct ConcurrentToken(LimitRx);

impl Drop for ConcurrentToken {
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

    fn acquire(&self) -> Result<ConcurrentToken> {
        self.tx.send(())?;

        Ok(ConcurrentToken(self.rx.clone()))
    }
}

/// StaggeredLimit is used to avoid the multiple concurrent tasks
/// that start at the same time, causing excessive strain
/// on resources such as cpu or disk. it makes multiple tasks to start at staggered times.
struct StaggeredLimit {
    token_tx: Sender<()>,
    task_done_tx: Sender<()>,
}

impl StaggeredLimit {
    /// Create a StaggeredLimit with specified concurrent and specified interval.
    ///
    /// * `concurrent` - The number of tasks that are allowed to start at the same time
    /// * `interval` - Time interval for task start
    fn new(concurrent: usize, interval: Duration) -> Self {
        let (token_tx, token_rx) = bounded(concurrent);
        let (task_done_tx, task_done_rx) = bounded(concurrent);

        // spawn the token filling delay task
        std::thread::spawn(move || {
            let send_token = || {
                match token_rx.try_recv() {
                    // we don't care the channel is empty or not.
                    Err(TryRecvError::Empty) => Ok(()),
                    x => x,
                }
            };
            let res = (|| -> Result<()> {
                loop {
                    let ticker = tick(interval);
                    loop {
                        select! {
                            recv(ticker) -> res => {
                                res?;
                                send_token()?;
                            },
                            recv(task_done_rx) -> res => {
                                res?;
                                send_token()?;
                                break;
                            },
                        }
                    }
                }
            })();
            if let Err(e) = res {
                warn!(err=?e, "StaggeredLimit channel disconnected");
            }
        });

        Self { token_tx, task_done_tx }
    }

    fn acquire(&self) -> Result<StaggeredToken> {
        self.token_tx
            .send(())
            .map(|_| StaggeredToken {
                task_done_tx: self.task_done_tx.clone(),
            })
            .map_err(Into::into)
    }
}

struct StaggeredToken {
    task_done_tx: Sender<()>,
}

impl Drop for StaggeredToken {
    fn drop(&mut self) {
        // it's safe to ignore the error here,
        // because we don't care the channel is full or not.
        let _ = self.task_done_tx.try_send(());
    }
}

/// Token combines ConcurrentToken and StaggeredToken
pub struct Token(Option<ConcurrentToken>, Option<StaggeredToken>);

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
                let mut token = Token(None, None);
                if let Some(concurrent_limit) = concurrent_limit_opt {
                    token.0 = Some(concurrent_limit.acquire()?)
                }

                if let Some(staggered_limit) = staggered_limit_opt {
                    token.1 = Some(staggered_limit.acquire()?);
                }
                debug!(target: LOG_TARGET, name = key, "acquired");
                Some(token)
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use humantime::format_duration;

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
    fn test_acquire_without_limit() {
        let pool = Pool::new(vec![].into_iter());

        let mut now = Instant::now();
        assert!(pool.acquire("pc1").unwrap().is_none());
        assert_elapsed!(now, ms(0), "without limit should not block");

        now = Instant::now();
        assert!(pool.acquire("pc1").unwrap().is_none());
        assert_elapsed!(now, ms(0), "without limit should not block");
    }

    #[test]
    fn test_acquire_both_concurrent_limit_and_staggered_limit() {
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
        timeout(ms(5), move || drop(token1));
        let _token3 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(5), "concurrent is only 2 so must wait for the `token1` to be dropped");
    }

    #[test]
    fn test_acquire_only_concurrent_limit() {
        let pool = Pool::new(
            (vec![LimitItem {
                name: "pc1",
                concurrent: Some(&3),
                staggered_interval: None,
            }])
            .into_iter(),
        );

        let mut now = Instant::now();
        let token1 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "first acquire should not block");

        now = Instant::now();
        let token2 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "seconed acquire should not block");

        now = Instant::now();
        let token3 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "third acquire should not block");

        now = Instant::now();
        timeout(ms(20), move || drop(token1));
        let token4 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(20), "concurrent is only 3 so must wait for the `token1` to be dropped");

        drop(token2);
        drop(token3);
        drop(token4);
        now = Instant::now();
        pool.acquire("pc1").unwrap();
        pool.acquire("pc1").unwrap();
        pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "should not block");
    }

    #[test]
    fn test_acquire_only_staggered_limit() {
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
        let _token3 = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval,
            "because of the staggered limit, it should to block for {}",
            format_duration(staggered_interval).to_string()
        );

        now = Instant::now();
        drop(token1);
        let _token4 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "since task1 has completed (token1 dropped), it should not be blocked");
    }

    #[test]
    fn test_a_task_done_acquire() {
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
        let token1 = pool.acquire("pc1").unwrap();
        assert_elapsed!(now, ms(0), "first acquire should not block");

        now = Instant::now();
        timeout(staggered_interval / 2, || drop(token1));
        let _token2 = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval / 2,
            "since task1 has completed (token1 dropped), it should only block for {}",
            format_duration(staggered_interval / 2).to_string()
        );

        now = Instant::now();
        let _token3 = pool.acquire("pc1").unwrap();
        assert_elapsed!(
            now,
            staggered_interval,
            "should block {}",
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
