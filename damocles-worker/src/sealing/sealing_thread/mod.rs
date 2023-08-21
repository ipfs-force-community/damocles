use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam_channel::{select, TryRecvError};

use crate::config::{Sealing, SealingOptional};
use crate::limit::SealingLimit;
use crate::logging::{error, info, warn};
use crate::store::{Location, Store};
use crate::watchdog::{Ctx, Module};

use super::config::{merge_sealing_fields, Config};
use super::failure::*;

mod planner;

pub use planner::default_plan;

pub mod entry;
#[macro_use]
mod util;

use util::*;

mod ctrl;

use ctrl::*;
pub use ctrl::{Ctrl, CtrlProcessor, SealingThreadState};

pub trait Sealer {
    fn seal(&mut self, state: Option<&str>) -> Result<R, Failure>;
}

pub enum R {
    SwitchPlanner(String),
    #[allow(dead_code)]
    Wait(Duration),
    Done,
}

#[derive(Clone)]
pub(crate) struct SealingCtrl<'a> {
    ctx: &'a Ctx,
    ctrl_ctx: &'a CtrlCtx,
    sealing_config: &'a Config,
}

impl<'a> SealingCtrl<'a> {
    pub fn config(&self) -> &Config {
        self.sealing_config
    }

    pub fn ctx(&self) -> &Ctx {
        self.ctx
    }

    pub fn ctrl_ctx(&self) -> &CtrlCtx {
        self.ctrl_ctx
    }

    pub fn interrupted(&self) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into())
            }

            default => {
                Ok(())
            }
        }
    }

    pub fn wait_or_interrupted(&self, duration: Duration) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into())
            }

            default(duration) => {
                Ok(())
            }
        }
    }
}

pub struct SealingThread {
    idx: usize,

    /// the config of this SealingThread
    pub config: Config,
    location: Option<Location>,

    ctrl_ctx: CtrlCtx,
}

impl SealingThread {
    pub fn new(
        idx: usize,
        plan: Option<String>,
        sealing_config: Sealing,
        location: Option<Location>,
        limit: Arc<SealingLimit>,
    ) -> Result<(Self, Ctrl)> {
        let (ctrl, ctrl_ctx) = new_ctrl(location.clone(), limit);

        Ok((
            Self {
                idx,
                config: Config::new(sealing_config, plan, location.as_ref().map(|x| x.hot_config_path()))?,
                location,
                ctrl_ctx,
            },
            ctrl,
        ))
    }

    fn seal_one(&mut self, ctx: &Ctx, state: Option<String>) -> Result<(), Failure> {
        self.config
            .reload_if_needed(|_, _| Ok(true))
            .context("reload sealing thread hot config")
            .crit()?;
        let mut plan = self.config.plan().to_string();
        loop {
            self.ctrl_ctx
                .update_state(|cst| cst.job.plan = plan.clone())
                .context("update ctrl state")
                .crit()?;
            let mut sealer = planner::create_sealer(&plan, ctx, self).crit()?;
            match sealer.seal(state.as_deref())? {
                R::SwitchPlanner(new_plan) => {
                    tracing::info!(new_plan = new_plan, "switch planner");
                    plan = new_plan;
                }
                R::Wait(dur) => self.sealing_ctrl(ctx).wait_or_interrupted(dur)?,
                R::Done => return Ok(()),
            }
        }
    }

    fn sealing_ctrl(&self, ctx: &Ctx) -> SealingCtrl<'static> {
        unsafe {
            SealingCtrl {
                ctx: extend_lifetime(ctx),
                ctrl_ctx: extend_lifetime(&self.ctrl_ctx),
                sealing_config: extend_lifetime(&self.config),
            }
        }
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
                            cst.state = SealingThreadState::Idle;
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
                            cst.state = SealingThreadState::PausedAt(Instant::now());
                            if !is_interrupt {
                                cst.job.last_error.replace(format!("{:?}", failure));
                            }
                        })?;
                        continue 'SEAL_LOOP;
                    }

                    Level::Abort => {
                        error!(?failure, "an abort level error occurred");
                    }
                };
            }

            info!(
                duration = ?self.config.seal_interval,
                "wait before sealing"
            );

            self.ctrl_ctx.update_state(|cst| {
                cst.job.id.take();
                cst.job.state = None;
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
    limit: Arc<SealingLimit>,
) -> Result<Vec<(SealingThread, (usize, Ctrl))>> {
    let mut sealing_threads = Vec::new();
    let mut path_set = HashSet::new();

    for (idx, scfg) in list.iter().enumerate() {
        let sealing_config = customized_sealing_config(common, scfg.inner.sealing.as_ref());
        let plan = scfg.inner.plan.as_ref().cloned();

        let loc = match &scfg.location {
            Some(loc) => {
                let store_path = PathBuf::from(loc);

                if !store_path.exists() {
                    Store::init(&store_path, false).with_context(|| format!("init store path: {}", store_path.display()))?;
                    info!(loc = ?store_path, "store initialized");
                }

                let store_path = store_path
                    .canonicalize()
                    .with_context(|| format!("canonicalize store path {}", loc))?;
                if path_set.contains(&store_path) {
                    tracing::warn!(path = ?store_path, "store already loaded");
                    continue;
                }
                path_set.insert(store_path.clone());

                Some(Location::new(store_path))
            }
            None => None,
        };

        let (sealing_thread, ctrl) = SealingThread::new(idx, plan, sealing_config, loc, limit.clone())?;
        sealing_threads.push((sealing_thread, (idx, ctrl)));
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

/// The caller is responsible for ensuring lifetime validity
pub const unsafe fn extend_lifetime<'b, T>(inp: &T) -> &'b T {
    std::mem::transmute(inp)
}

/// The caller is responsible for ensuring lifetime validity
#[allow(dead_code)]
pub unsafe fn extend_lifetime_mut<'b, T>(inp: &mut T) -> &'b mut T {
    std::mem::transmute(inp)
}
