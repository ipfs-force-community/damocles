use std::collections::HashSet;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam_channel::{select, TryRecvError};

use crate::config::{Sealing, SealingOptional};
use crate::logging::{error, info, warn};
use crate::store::Location;
use crate::watchdog::{Ctx, Module};

use super::config::{merge_sealing_fields, Config};
use super::{failure::*, store::Store};

mod task;
use task::{sector::State, Task};

mod ctrl;
pub use ctrl::Ctrl;
use ctrl::*;

pub struct SealingThread {
    idx: usize,

    /// the config of this SealingThread
    pub config: Config,
    pub store: Store,

    ctrl_ctx: CtrlCtx,
}

impl SealingThread {
    pub fn new(idx: usize, plan: Option<String>, sealing_config: Sealing, location: Location) -> Result<(Self, Ctrl)> {
        let store_path = location.to_pathbuf();
        let store = Store::open(store_path).with_context(|| format!("open store {}", location.as_ref().display()))?;
        let (ctrl, ctrl_ctx) = new_ctrl(location.clone());

        Ok((
            Self {
                config: Config::new(sealing_config, plan, Some(location.hot_config_path()))?,
                store,
                ctrl_ctx,
                idx,
            },
            ctrl,
        ))
    }

    fn seal_one(&mut self, ctx: &Ctx, state: Option<State>) -> Result<(), Failure> {
        let task = Task::build(ctx, &self.ctrl_ctx, &mut self.config, &mut self.store)?;
        task.exec(state)
    }
}

impl Module for SealingThread {
    fn should_wait(&self) -> bool {
        false
    }

    fn id(&self) -> String {
        format!("worker-{}", self.idx)
    }

    fn run(&mut self, ctx: Ctx) -> Result<()> {
        let _rt_guard = ctx.global.rt.enter();

        let mut wait_for_resume = false;
        let mut resume_state = None;
        let resume_loop_tick = Duration::from_secs(60);

        'SEAL_LOOP: loop {
            if wait_for_resume {
                warn!("waiting for resume signal");

                select! {
                    recv(self.ctrl_ctx.resume_rx) -> resume_res => {
                        // resume sealing procedure with given SetState target
                        resume_state = resume_res.context("resume signal channel closed unexpectedly")?;

                        wait_for_resume = false;

                        self.ctrl_ctx.update_state(|cst| {
                            cst.paused_at.take();
                            cst.job.last_error.take();
                        })?;
                    },

                    recv(ctx.done) -> _done_res => {
                        return Ok(())
                    },

                    default(resume_loop_tick) => {
                        warn!("worker has been waiting for resume signal during the last {:?}", resume_loop_tick);
                        continue 'SEAL_LOOP
                    }
                }
            }

            if ctx.done.try_recv() != Err(TryRecvError::Empty) {
                return Ok(());
            }

            if let Err(failure) = self.seal_one(&ctx, resume_state.take()) {
                let is_interrupt = (failure.1).is::<Interrupt>();
                if !is_interrupt {
                    error!(?failure, "sealing failed");
                } else {
                    warn!("sealing interrupted");
                }

                match failure.0 {
                    Level::Temporary | Level::Permanent | Level::Critical => {
                        if failure.0 == Level::Temporary {
                            error!("temporary error should not be popagated to the top level");
                        };

                        wait_for_resume = true;

                        self.ctrl_ctx.update_state(|cst| {
                            cst.paused_at.replace(Instant::now());
                            if !is_interrupt {
                                cst.job.last_error.replace(format!("{:?}", failure));
                            }
                        })?;
                        continue 'SEAL_LOOP;
                    }

                    Level::Abort => {}
                };
            }

            info!(
                duration = ?self.config.seal_interval,
                "wait before sealing"
            );

            self.ctrl_ctx.update_state(|cst| {
                cst.job.id.take();
                let _ = std::mem::replace(&mut cst.job.state, State::Empty);
            })?;

            select! {
                recv(ctx.done) -> _done_res => {
                    return Ok(())
                },

                default(self.config.seal_interval) => {

                }
            }
        }
    }
}

pub(crate) fn build_sealing_threads(
    list: &[crate::config::SealingThread],
    common: &SealingOptional,
) -> Result<Vec<(SealingThread, (usize, Ctrl))>> {
    let mut sealing_threads = Vec::new();
    let mut path_set = HashSet::new();

    for (idx, scfg) in list.iter().enumerate() {
        let sealing_config = customized_sealing_config(common, scfg.inner.sealing.as_ref());
        let plan = scfg.inner.plan.as_ref().cloned();

        let store_path = PathBuf::from(&scfg.location)
            .canonicalize()
            .with_context(|| format!("canonicalize store path {}", &scfg.location))?;
        if path_set.contains(&store_path) {
            tracing::warn!(path = ?store_path, "store already loaded");
            continue;
        }

        let (sealing_thread, ctrl) = SealingThread::new(idx, plan, sealing_config, Location::new(store_path.clone()))?;
        sealing_threads.push((sealing_thread, (idx, ctrl)));
        path_set.insert(store_path);
    }

    Ok(sealing_threads)
}

fn customized_sealing_config(common: &SealingOptional, customized: Option<&SealingOptional>) -> Sealing {
    let default_sealing = Sealing::default();
    let common_sealing = merge_sealing_fields(default_sealing, common.clone());
    if let Some(customized) = customized.cloned() {
        merge_sealing_fields(common_sealing, customized)
    } else {
        common_sealing
    }
}
