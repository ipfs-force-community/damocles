//! external implementations of processors

use std::collections::HashMap;
use std::env::current_exe;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, Receiver, Sender, TrySendError};
use rand::rngs::OsRng;
use rand::seq::SliceRandom;
use tokio::sync::Semaphore;
use tower::limit::{ConcurrencyLimit, ConcurrencyLimitLayer};
use tower::util::Either;
use tower::ServiceBuilder;
use vc_fil_consumers::TaskId;
use vc_processors::middleware::limit::delay::{Delay, DelayLayer};
use vc_processors::middleware::limit::lock::{Lock, LockLayer};
use vc_processors::producer::Producer;
use vc_processors::sys::cgroup;
use vc_processors::transport::default::{connect, pipe};
use vc_processors::util::tower::TowerWrapper;
use vc_processors::util::ProcessorExt;
use vc_processors::{ready_msg, ProcessorClient, Task};

use self::config::Ext;

pub mod config;

const DEFAULT_CGROUP_GROUP_NAME: &str = "vc-worker";

type OptDelay<T> = Either<Delay<TowerWrapper<Producer<T, u64>>>, TowerWrapper<Producer<T, u64>>>;
type OptLock<T> = Either<Lock<OptDelay<T>>, OptDelay<T>>;
type OptConcurrency<T> = Either<ConcurrencyLimit<OptLock<T>>, OptLock<T>>;

type ChildService<Tsk> = OptConcurrency<Tsk>;

#[derive(Debug, Clone)]
pub struct ExtProcessor<Tsk: Task> {
    limit: Option<ConcurrentLimit>,
    subs: Vec<SubProcessor<Tsk>>,
}

impl<Tsk: Task> ExtProcessor<Tsk> {
    pub fn build(
        cfg: &[config::Ext],
        delay: Option<Duration>,
        concurrent: Option<usize>,
        ext_locks: &HashMap<String, Arc<Semaphore>>,
    ) -> Result<Self> {
        let mut subs = Vec::with_capacity(cfg.len());
        for (index, c) in cfg.iter().enumerate() {
            let svc = make(index, c, delay, ext_locks).with_context(|| format!("start sub process for stage {}", Tsk::STAGE))?;
            subs.push(SubProcessor {
                weight: c.weight,
                producer: ProcessorClient::new(svc),
            });
        }

        Ok(Self {
            limit: concurrent.map(ConcurrentLimit::new),
            subs,
        })
    }
}

impl<T: Task> super::Client<T> for ExtProcessor<T> {
    fn process(&mut self, task: T) -> anyhow::Result<<T as Task>::Output> {
        let _token = self.limit.as_ref().map(|limit| limit.acquire()).transpose()?;

        let chosen = self
            .subs
            .choose_weighted_mut(&mut OsRng, |x| x.weight)
            .context("no input tx from available chosen")?;
        crate::block_on(chosen.producer.process(task)).map_err(|e| anyhow!(e))
    }
}

#[derive(Debug, Clone)]
pub(super) struct SubProcessor<Tsk: Task> {
    pub weight: u16,
    pub producer: ProcessorClient<Tsk, ChildService<Tsk>>,
}

fn make<T: Task>(index: usize, cfg: &Ext, delay: Option<Duration>, ext_locks: &HashMap<String, Arc<Semaphore>>) -> Result<ChildService<T>> {
    let bin = match &cfg.bin {
        Some(x) => PathBuf::from(x),
        None => current_exe().context("get current exe path")?,
    };
    let mut cmd = pipe::Command::new(bin)
        .args(
            cfg.args
                .clone()
                .unwrap_or_else(|| vec!["processor".to_string(), T::STAGE.to_string()]),
        )
        .auto_restart(cfg.auto_restart)
        .ready_message(ready_msg::<T>())
        .ready_timeout(cfg.stable_wait);

    if let Some(envs) = cfg.envs.as_ref() {
        cmd = cmd.envs(envs)
    }

    #[cfg(not(target_os = "macos"))]
    if let Some(preferred) = cfg.numa_preferred {
        cmd = vc_processors::sys::numa::pipe_set_preferred(cmd, preferred);
    }

    if let Some(cgroup) = &cfg.cgroup {
        if let Some(cpuset) = cgroup.cpuset.as_ref() {
            let cgname = format!(
                "{}/sub-{}-{}-{}",
                cgroup.group_name.as_deref().unwrap_or(DEFAULT_CGROUP_GROUP_NAME),
                T::STAGE,
                std::process::id(),
                index
            );
            cmd = cgroup::pipe_set_cpuset(cmd, cgname, cpuset);
        }
    };

    let svc = ServiceBuilder::new()
            .option_layer(cfg.concurrent.map(ConcurrencyLimitLayer::new)) // set the concurrency of a single subprocess
            .option_layer(cfg.locks.as_ref()
                .map(|locks| {
                    locks.iter().filter_map(|lock| ext_locks.get(lock)).cloned().collect::<Vec<_>>()
                })
                .map(LockLayer::new)).option_layer(delay.map(DelayLayer::new))
            .service(Producer::<_, TaskId>::new(connect(cmd)).spawn().tower());
    Ok(svc)
}

/// Token represents 1 available resource unit
pub struct Token(Receiver<()>);

impl Drop for Token {
    fn drop(&mut self) {
        let _ = self.0.recv();
    }
}

#[derive(Debug, Clone)]
struct ConcurrentLimit {
    tx: Sender<()>,
    rx: Receiver<()>,
}

impl ConcurrentLimit {
    fn new(size: usize) -> Self {
        let (tx, rx) = bounded(size);

        ConcurrentLimit { tx, rx }
    }

    fn acquire(&self) -> Result<Token> {
        self.tx.send(()).map(|_| Token(self.rx.clone())).map_err(|_| Self::limiter_closed())
    }

    #[allow(dead_code)]
    fn try_acquire(&self) -> Result<Option<Token>> {
        match self.tx.try_send(()) {
            Ok(_) => Ok(Some(Token(self.rx.clone()))),
            Err(TrySendError::Full(_)) => Ok(None),
            Err(TrySendError::Disconnected(_)) => Err(Self::limiter_closed()),
        }
    }

    #[inline]
    fn limiter_closed() -> anyhow::Error {
        anyhow!("resource limiter closed")
    }
}
