//! resource control, acquiring & releasing

use std::{collections::HashMap, time::Duration};

use anyhow::{anyhow, Error, Result};
use crossbeam_channel::{bounded, Receiver, RecvTimeoutError, Sender, TryRecvError, TrySendError};

use crate::logging::{debug, warn};

const LOG_TARGET: &str = "resource";

type LimitTx = Sender<()>;
type LimitRx = Receiver<()>;

#[inline]
fn limiter_closed() -> Error {
    anyhow!("resource limiter closed")
}

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
        self.tx
            .send(())
            .map(|_| ConcurrentToken(self.rx.clone()))
            .map_err(|_| limiter_closed())
    }

    fn try_acquire(&self) -> Result<Option<ConcurrentToken>> {
        match self.tx.try_send(()) {
            Ok(_) => Ok(Some(ConcurrentToken(self.rx.clone()))),
            Err(TrySendError::Full(_)) => Ok(None),
            Err(TrySendError::Disconnected(_)) => Err(limiter_closed()),
        }
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
                    // empty channel means the token is full
                    Ok(_) | Err(TryRecvError::Empty) => Ok(()),
                    x @ Err(TryRecvError::Disconnected) => x,
                }
            };
            let res = (|| -> Result<(), TryRecvError> {
                loop {
                    match task_done_rx.recv_timeout(interval) {
                        Ok(_) | Err(RecvTimeoutError::Timeout) => send_token()?,
                        Err(RecvTimeoutError::Disconnected) => return Err(TryRecvError::Disconnected),
                    }
                }
            })();
            if res.is_err() {
                warn!(target: LOG_TARGET, "StaggeredLimit channel disconnected");
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
            .map_err(|_| limiter_closed())
    }

    fn try_acquire(&self) -> Result<Option<StaggeredToken>> {
        match self.token_tx.try_send(()) {
            Ok(_) => Ok(Some(StaggeredToken {
                task_done_tx: self.task_done_tx.clone(),
            })),
            Err(TrySendError::Full(_)) => Ok(None),
            Err(TrySendError::Disconnected(_)) => Err(limiter_closed()),
        }
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
/// Token is an RAII implementation of a "scoped lock" of a resource.
/// When this structure is dropped (falls out of scope), the resource will be unlocked.
pub struct Token(Option<ConcurrentToken>, Option<StaggeredToken>);

/// resource limit pool
pub struct Pool {
    pool: HashMap<String, (Option<ConcurrentLimit>, Option<StaggeredLimit>)>,
}

#[derive(Debug)]
pub struct LimitItem {
    pub name: String,
    pub concurrent: Option<usize>,
    pub staggered_interval: Option<Duration>,
}

impl Pool {
    #[cfg(not(test))]
    const MIN_STAGGERED_INTERVAL: Duration = Duration::from_secs(1);
    #[cfg(test)]
    const MIN_STAGGERED_INTERVAL: Duration = Duration::from_millis(1);

    pub fn empty() -> Self {
        Self { pool: HashMap::new() }
    }

    /// extend resources
    pub fn extend<I: Iterator<Item = LimitItem>>(&mut self, iter: I) {
        for limit_item in iter {
            debug!(
                target: LOG_TARGET,
                name = limit_item.name,
                concurrent = limit_item.concurrent,
                staggered_time_interval = limit_item.staggered_interval.unwrap_or_default().as_secs_f32(),
                "add limitation"
            );

            let concurrent_limit_opt = limit_item.concurrent.map(|concurrent| ConcurrentLimit::new(concurrent));
            let staggered_limit_opt = limit_item.staggered_interval.and_then(|interval| {
                if interval < Self::MIN_STAGGERED_INTERVAL {
                    warn!(staggered_interval = ?interval, "staggered interval must be greater than or equal to {:?}", Self::MIN_STAGGERED_INTERVAL);
                    None
                } else {
                    Some(StaggeredLimit::new(1, interval.to_owned()))
                }
            });
            self.pool.insert(limit_item.name, (concurrent_limit_opt, staggered_limit_opt));
        }
    }

    /// acquires a token for the named resource
    pub fn acquire<N: AsRef<str>>(&self, name: N) -> Result<Token> {
        let key = name.as_ref();
        Ok(match self.pool.get(key) {
            None | Some((None, None)) => {
                debug!(target: LOG_TARGET, name = key, "unlimited");
                Token(None, None)
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
                token
            }
        })
    }

    /// Attempts to acquire the named resource.
    ///
    /// If the resource is successfully acquire, an RAII guard `Token` is returned.
    /// The resource will be released when the guard `Token` is dropped.
    ///
    /// If the resource could not be acquired because it is already occupied,
    /// then this call will return the `Ok(None)`.
    ///
    /// Otherwise return an error.
    pub fn try_acquire<N: AsRef<str>>(&self, name: N) -> Result<Option<Token>> {
        let key = name.as_ref();
        Ok(match self.pool.get(key) {
            None | Some((None, None)) => {
                debug!(target: LOG_TARGET, name = key, "unlimited");
                Some(Token(None, None))
            }
            Some((concurrent_limit_opt, staggered_limit_opt)) => {
                let mut token = Token(None, None);
                if let Some(concurrent_limit) = concurrent_limit_opt {
                    match concurrent_limit.try_acquire()? {
                        t @ Some(_) => token.0 = t,
                        None => return Ok(None),
                    }
                }

                if let Some(staggered_limit_opt) = staggered_limit_opt {
                    match staggered_limit_opt.try_acquire()? {
                        t @ Some(_) => token.1 = t,
                        None => return Ok(None),
                    }
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
            let mut lower: std::time::Duration = $dur;

            // Handles ms rounding
            if lower > ms(1) {
                lower -= ms(1)
            }
            assert!(
                elapsed >= lower,
                "actual = {:?}, expected = {:?}. {}",
                elapsed,
                lower,
                format_args!($($arg)+),
            );
        }};
    }

    #[test]
    fn test_acquire_without_limit() {
        // time::pause();
        let pool = Pool::empty();

        let mut now = Instant::now();
        let token1 = pool.acquire("pc1").unwrap();
        assert!(token1.0.is_none());
        assert!(token1.1.is_none());
        assert_elapsed!(now, ms(0), "without limit should not block");

        now = Instant::now();
        let token2 = pool.acquire("pc1").unwrap();
        assert!(token2.0.is_none());
        assert!(token2.1.is_none());
        assert_elapsed!(now, ms(0), "without limit should not block");
    }

    #[test]
    fn test_acquire_both_concurrent_limit_and_staggered_limit() {
        let staggered_interval = ms(10);
        let mut pool = Pool::empty();
        pool.extend(
            (vec![LimitItem {
                name: "pc1".to_string(),
                concurrent: Some(2),
                staggered_interval: Some(staggered_interval),
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
        let mut pool = Pool::empty();
        pool.extend(
            (vec![LimitItem {
                name: "pc1".to_string(),
                concurrent: Some(3),
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
        let mut pool = Pool::empty();
        pool.extend(
            (vec![LimitItem {
                name: "pc1".to_string(),
                concurrent: None,
                staggered_interval: Some(staggered_interval),
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
        let mut pool = Pool::empty();
        pool.extend(
            (vec![LimitItem {
                name: "pc1".to_string(),
                concurrent: None,
                staggered_interval: Some(staggered_interval),
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

    #[test]
    fn test_try_acquire() {
        let mut pool = Pool::empty();
        pool.extend(
            (vec![LimitItem {
                name: "pc1".to_string(),
                concurrent: Some(1),
                staggered_interval: None,
            }])
            .into_iter(),
        );
        let token1 = pool.try_acquire("pc1").unwrap();
        assert!(token1.is_some(), "first try_acquire should not block");
        assert!(pool.try_acquire("pc1").unwrap().is_none(), "second acquire should block");
        drop(token1);
        assert!(pool.try_acquire("pc1").unwrap().is_some(), "should not block");
    }

    fn ms(n: u64) -> Duration {
        Duration::from_millis(n)
    }

    fn timeout(t: Duration, f: impl FnOnce() + Send + 'static) {
        let _ = std::thread::spawn(move || {
            std::thread::sleep(t);
            f();
        });
    }
}
