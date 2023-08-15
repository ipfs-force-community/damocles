//! external implementations of processors

use std::sync::Arc;

use anyhow::{anyhow, Result};
use vc_processors::core::Task;

use self::{concurrent::Concurrent, ext_locks::ExtLocks, sub::SubProcessor, weight::Weight};

use crate::limit::SealingLimit;

mod concurrent;
pub mod config;
mod ext_locks;
pub mod sub;
pub mod weight;

pub type Proc<T> = Weight<ExtLocks<Concurrent<SubProcessor<T>>>>;

pub fn start_sub_processors<T: Task>(cfgs: &[config::Ext], limit: Arc<SealingLimit>) -> Result<Proc<T>> {
    if cfgs.is_empty() {
        return Err(anyhow!("no subs section found"));
    }

    let mut procs = Vec::with_capacity(cfgs.len());
    for (index, sub_cfg) in cfgs.iter().enumerate() {
        let subprocessor = SubProcessor::<T>::new(index, sub_cfg)?;
        procs.push(ExtLocks::new(
            Concurrent::new(subprocessor, sub_cfg.concurrent),
            sub_cfg.locks.as_ref().cloned().unwrap_or_default(),
            limit.clone(),
        ));
    }

    Ok(Weight::new(procs))
}
