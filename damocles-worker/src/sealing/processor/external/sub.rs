//! utilities for managing sub processes of processor

use std::{env::current_exe, path::PathBuf};

use anyhow::{Context, Ok, Result};
use vc_processors::core::{ext::ProducerBuilder, Processor, Task as Input};

use super::{
    config,
    weight::{TryLockProcessor, Weighted},
};
use crate::sealing::processor::LockProcessor;

const DEFAULT_CGROUP_GROUP_NAME: &str = "vc-worker";

pub struct SubProcessor<I: Input> {
    weight: u16,
    producer: Box<dyn Processor<I>>,
}

impl<I: Input> SubProcessor<I> {
    pub fn new(index: usize, sub_cfg: &config::Ext) -> Result<Self> {
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
            .unwrap_or_else(|| vec!["processor".to_owned(), I::STAGE.to_owned()]);

        let mut builder = ProducerBuilder::new(bin, args)
            .inherit_envs(true)
            .stable_timeout(sub_cfg.stable_wait)
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
                    I::STAGE,
                    std::process::id(),
                    index
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

        Ok(Self {
            weight: sub_cfg.weight,
            producer: Box::new(producer),
        })
    }
}

impl<I: Input> LockProcessor for SubProcessor<I> {
    type Guard<'a> = &'a Box<dyn Processor<I>>
    where
        Self: 'a;

    fn lock(&self) -> Result<Self::Guard<'_>> {
        Ok(&self.producer)
    }
}

impl<I: Input> TryLockProcessor for SubProcessor<I> {
    fn try_lock(&self) -> Result<Option<Self::Guard<'_>>> {
        Ok(Some(&self.producer))
    }
}

impl<I: Input> Weighted for SubProcessor<I> {
    fn weight(&self) -> u16 {
        self.weight
    }
}
