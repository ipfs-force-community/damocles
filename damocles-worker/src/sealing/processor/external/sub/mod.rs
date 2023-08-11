//! utilities for managing sub processes of processor

use std::env::current_exe;
use std::path::PathBuf;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, unbounded, Sender};
use tracing::debug;
use vc_processors::core::{
    ext::{ProducerBuilder, Request},
    Processor,
};

use super::{super::Input, config};
use crate::sealing::resource::{self, Pool};

const DEFAULT_CGROUP_GROUP_NAME: &str = "vc-worker";

pub(super) struct SubProcessor<I: Input> {
    pub limiter: Sender<()>,
    pub locks: Vec<String>,
    pub weight: u16,
    pub producer: Box<dyn Processor<I>>,
}

impl<I: Input> SubProcessor<I> {
    pub fn try_lock(&self, res_limit_pool: &Pool) -> Result<Option<ProcessingGuard>> {
        let mut tokens = Vec::new();
        for lock_name in &self.locks {
            debug!(name = lock_name.as_str(), stage = I::STAGE, "acquiring lock");

            match res_limit_pool.try_acquire(lock_name)? {
                Some(t) => tokens.push(t),
                None => return Ok(None),
            }
        }

        Ok(Some(ProcessingGuard(tokens)))
    }

    pub fn lock(&self, res_limit_pool: &Pool) -> Result<ProcessingGuard> {
        let mut tokens = Vec::new();
        for lock_name in &self.locks {
            tokens.push(res_limit_pool.acquire(lock_name)?);
        }
        Ok(ProcessingGuard(tokens))
    }
}

pub(crate) struct ProcessingGuard(Vec<resource::Token>);

pub(super) fn start_sub_processors<I: Input>(cfgs: &[config::Ext]) -> Result<Vec<SubProcessor<I>>> {
    if cfgs.is_empty() {
        return Err(anyhow!("no subs section found"));
    }

    let mut procs = Vec::with_capacity(cfgs.len());
    let stage = I::STAGE;

    for (i, sub_cfg) in cfgs.iter().enumerate() {
        let (limit_tx, limit_rx) = match sub_cfg.concurrent {
            Some(0) => return Err(anyhow!("invalid concurrent limit 0")),
            Some(size) => bounded(size),
            None => unbounded(),
        };

        let bin = sub_cfg
            .bin
            .as_ref()
            .cloned()
            .map(|s| Ok(PathBuf::from(s)))
            .unwrap_or_else(|| current_exe().context("get current exe name"))?;

        let args = sub_cfg
            .args
            .as_ref()
            .cloned()
            .unwrap_or_else(|| vec!["processor".to_owned(), stage.to_owned()]);

        let hook_limit_tx = limit_tx.clone();
        let mut builder = ProducerBuilder::new(bin, args)
            .inherit_envs(true)
            .stable_timeout(sub_cfg.stable_wait)
            .hook_prepare(move |_: &Request<I>| -> Result<()> { hook_limit_tx.send(()).context("limit chan broken") })
            .hook_finalize(move |_: &Request<I>| {
                // check this?
                let _ = limit_rx.try_recv();
            })
            .auto_restart(sub_cfg.auto_restart);

        #[cfg(not(target_os = "macos"))]
        if let Some(preferred) = sub_cfg.numa_preferred {
            builder = builder.numa_preferred(preferred);
        }
        if let Some(cgroup) = &sub_cfg.cgroup {
            if let Some(cpuset) = cgroup.cpuset.as_ref() {
                let cgname = format!(
                    "{}/sub-{}-{}-{}",
                    cgroup.group_name.as_deref().unwrap_or(DEFAULT_CGROUP_GROUP_NAME),
                    stage,
                    std::process::id(),
                    i
                );
                builder = builder.cpuset(cgname, cpuset.to_string());
            }
        }

        if let Some(envs) = sub_cfg.envs.as_ref() {
            for (k, v) in envs {
                builder = builder.env(k.to_owned(), v.to_owned());
            }
        }

        let producer = builder.spawn::<I>().context("build ext producer")?;

        procs.push(SubProcessor {
            limiter: limit_tx,
            locks: sub_cfg.locks.as_ref().cloned().unwrap_or_default(),
            weight: sub_cfg.weight,
            producer: Box::new(producer),
        });
    }

    Ok(procs)
}
