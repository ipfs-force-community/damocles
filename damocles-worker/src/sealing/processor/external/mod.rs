//! external implementations of processors

use std::sync::Arc;

use anyhow::{anyhow, Result};
use vc_processors::core::Task;

use self::{
    concurrent::Concurrent,
    ext_locks::ExtLocks,
    load::{Workload, WorkloadSelector},
    sub::SubProcessor,
};

use crate::limit::SealingLimit;

mod concurrent;
pub mod config;
mod ext_locks;
mod load;
mod sub;

pub type Proc<T> = WorkloadSelector<Workload<ExtLocks<Concurrent<SubProcessor<T>>>>>;

pub fn start_sub_processors<T: Task>(cfgs: &[config::Ext], limit: Arc<SealingLimit>) -> Result<Proc<T>> {
    if cfgs.is_empty() {
        return Err(anyhow!("no subs section found"));
    }

    let mut procs = Vec::with_capacity(cfgs.len());
    for (index, sub_cfg) in cfgs.iter().enumerate() {
        let subprocessor = SubProcessor::<T>::new(index, sub_cfg)?;
        procs.push(Workload::new(
            ExtLocks::new(
                Concurrent::new(subprocessor, sub_cfg.concurrent),
                sub_cfg.locks.as_ref().cloned().unwrap_or_default(),
                limit.clone(),
            ),
            sub_cfg.concurrent.map(|x| x as u32),
        ));
    }

    Ok(WorkloadSelector::new(procs))
}
