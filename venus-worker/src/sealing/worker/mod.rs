use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam_channel::{select, TryRecvError};

use crate::logging::{error, info, warn};
use crate::watchdog::{Ctx, Module};

use super::{failure::*, store::Store};

mod task;
use task::{sector::State, Task};

mod ctrl;
pub use ctrl::Ctrl;
use ctrl::*;

pub struct Worker {
    idx: usize,
    store: Store,
    ctrl_ctx: CtrlCtx,
}

impl Worker {
    pub fn new(idx: usize, s: Store) -> (Self, Ctrl) {
        let (ctrl, ctrl_ctx) = new_ctrl(s.location.clone());
        (
            Worker {
                idx,
                store: s,
                ctrl_ctx,
            },
            ctrl,
        )
    }

    fn seal_one(&mut self, ctx: &Ctx, state: Option<State>) -> Result<(), Failure> {
        let task = Task::build(ctx, &self.ctrl_ctx, &self.store)?;
        task.exec(state)
    }
}

impl Module for Worker {
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
                    warn!("sealing interruptted");
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
                duration = ?self.store.config.seal_interval,
                "wait before sealing"
            );

            self.ctrl_ctx.update_state(|cst| {
                cst.job.id.take();
                drop(std::mem::replace(&mut cst.job.state, State::Empty));
            })?;

            select! {
                recv(ctx.done) -> _done_res => {
                    return Ok(())
                },

                default(self.store.config.seal_interval) => {

                }
            }
        }
    }
}
