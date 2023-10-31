//! this module provides some common handlers

pub(crate) mod event;
pub(crate) mod sealing;
pub(crate) mod sector;
pub(crate) mod task;

use std::{pin::Pin, str::FromStr};

use anyhow::{anyhow, Context, Result};
pub use sealing::*;

use crate::sealing::failure::TaskAborted;
use crate::{
    rpc::sealer::{SectorFailure, SectorStateChange},
    sealing::{
        failure::{
            Failure, FailureContext, IntoFailure, Level, MapErrToFailure,
        },
        sealing_thread::{
            extend_lifetime, planner::JobTrait, Sealer, SealingThread, R,
        },
    },
    store::Store,
    watchdog::Ctx,
};

use self::{event::Event, sector::State, task::Task};

use super::PlannerTrait;

pub(crate) struct CommonSealer<P> {
    job: Task,
    planner: P,
    _store: Pin<Box<Store>>,
}

impl<P> Sealer for CommonSealer<P>
where
    P: PlannerTrait<Job = Task, Event = Event, State = State>,
{
    fn seal(&mut self, state: Option<&str>) -> Result<R, Failure> {
        let mut event = state
            .and_then(|s| State::from_str(s).ok())
            .map(Event::SetState);
        if let (true, Some(s)) = (event.is_none(), state) {
            tracing::error!("unknown state: {}", s);
        }

        let mut task_idle_count = 0;
        loop {
            let span = tracing::error_span!(
                "seal",
                miner = ?self.job.sector.base.as_ref().map(|b| b.allocated.id.miner),
                sector = ?self.job.sector.base.as_ref().map(|b| b.allocated.id.number),
                ?event,
            );

            let _enter = span.enter();

            let prev = self.job.sector.state;
            let is_empty = match self.job.sector.base.as_ref() {
                None => true,
                Some(base) => {
                    self.job
                        .sealing_ctrl
                        .ctrl_ctx()
                        .update_state(|cst| {
                            cst.job.id.replace(base.allocated.id.to_string());
                        })
                        .crit()?;
                    false
                }
            };

            if self.job.sector.plan() != self.planner.name() {
                return Ok(R::SwitchPlanner(
                    self.job.sector.plan().to_string(),
                ));
            }

            self.job.sealing_ctrl.interrupted()?;

            let handle_res = self.handle(event.take());
            if is_empty {
                if let Some(base) = self.job.sector.base.as_ref() {
                    self.job
                        .sealing_ctrl
                        .ctrl_ctx()
                        .update_state(|cst| {
                            cst.job.id.replace(base.allocated.id.to_string());
                        })
                        .crit()?;
                }
            } else if self.job.sector.base.is_none() {
                self.job
                    .sealing_ctrl
                    .ctrl_ctx()
                    .update_state(|cst| {
                        cst.job.id.take();
                    })
                    .crit()?;
            }

            let fail = if let Err(eref) = handle_res.as_ref() {
                Some(SectorFailure {
                    level: format!("{:?}", eref.0),
                    desc: format!("{:?}", eref.1),
                })
            } else {
                None
            };

            match self.job.report_state(
                SectorStateChange {
                    prev: prev.as_str().to_owned(),
                    next: self.job.sector.state.as_str().to_owned(),
                    event: format!("{:?}", event),
                },
                fail,
            ) {
                Err(rerr) => {
                    tracing::error!("report state failed: {:?}", rerr);
                }
                Ok(state) => {
                    if state.finalized {
                        tracing::warn!("cleanup aborted sector");
                        self.job.finalize()?;
                        match state.abort_reason {
                            None => return Err(TaskAborted.into()),
                            Some(reason) => {
                                return Err(Failure(
                                    Level::Abort,
                                    anyhow!(reason),
                                ))
                            }
                        }
                    }
                }
            }

            match handle_res {
                Ok(Some(evt)) => {
                    if let Event::Idle = &evt {
                        task_idle_count += 1;
                        if task_idle_count
                            > self
                                .job
                                .sealing_ctrl
                                .config()
                                .request_task_max_retries
                        {
                            tracing::info!(
                                "The task has returned `Event::Idle` for more than {} times. break the task",
                                self.job.sealing_ctrl.config().request_task_max_retries
                            );

                            // when the planner tries to request a task but fails(including no task) for more than
                            // `config::sealing::request_task_max_retries` times, this task is really considered idle,
                            // break this task loop. that we have a chance to reload `sealing_thread` hot config file,
                            // or do something else.

                            if self.job.sealing_ctrl.config().check_modified() {
                                // cleanup sector if the hot config modified
                                self.job.finalize()?;
                            }
                            return Ok(R::Done);
                        }
                    }
                    event.replace(evt);
                }

                Ok(None) => match self
                    .job
                    .report_finalized()
                    .context("report finalized")
                {
                    Ok(_) => {
                        self.job.finalize()?;
                        return Ok(R::Done);
                    }
                    Err(terr) => self.job.retry(terr.1)?,
                },

                Err(Failure(Level::Abort, aerr)) => {
                    if let Err(rerr) =
                        self.job.report_aborted(format!("{:#}", aerr))
                    {
                        tracing::error!(
                            "report aborted sector failed: {:?}",
                            rerr
                        );
                    }

                    tracing::warn!("cleanup aborted sector");
                    self.job.finalize()?;
                    return Err(aerr.abort());
                }

                Err(Failure(Level::Temporary, terr)) => self.job.retry(terr)?,

                Err(f) => return Err(f),
            }
        }
    }
}

impl<P> CommonSealer<P>
where
    P: PlannerTrait<Job = Task, Event = Event, State = State>,
{
    pub fn new(ctx: &Ctx, st: &SealingThread) -> Result<Self>
    where
        Self: Sized,
    {
        let location = st.location.as_ref().expect("location must be set");
        let store_path = location.to_pathbuf();
        let store = Box::pin(Store::open(store_path).with_context(|| {
            format!("open store {}", location.as_ref().display())
        })?);

        Ok(Self {
            job: Task::build(st.sealing_ctrl(ctx), unsafe {
                extend_lifetime(&*store.as_ref())
            })
            .context("build tesk")?,
            planner: P::default(),
            _store: store,
        })
    }

    fn handle(
        &mut self,
        event: Option<Event>,
    ) -> Result<Option<Event>, Failure> {
        let prev = self.job.sector.state;

        if let Some(evt) = event {
            match &evt {
                Event::Idle | Event::Retry => {
                    tracing::debug!(
                        prev = ?self.job.sector.state,
                        sleep = ?self.job.sealing_ctrl.config().recover_interval,
                        "Event::{:?} captured", evt
                    );

                    self.job.sealing_ctrl.wait_or_interrupted(
                        self.job.sealing_ctrl.config().recover_interval,
                    )?;
                }

                other => {
                    let next = if let Event::SetState(s) = other {
                        *s
                    } else {
                        self.planner
                            .plan(other, &self.job.sector.state)
                            .crit()?
                    };
                    self.planner
                        .apply(evt, next, &mut self.job)
                        .context("event apply")
                        .crit()?;
                    self.job.sector.sync().context("sync sector").crit()?;
                }
            };
        };

        let span = tracing::warn_span!("handle", ?prev, current = ?self.job.sector.state);

        let _enter = span.enter();

        self.job
            .sealing_ctrl
            .ctrl_ctx()
            .update_state(|cst| {
                cst.job.id = self
                    .job
                    .sector
                    .base
                    .as_ref()
                    .map(|x| x.allocated.id.to_string());
                let _ = cst
                    .job
                    .state
                    .replace(self.job.sector.state.as_str().to_string());
                let _ = cst.job.stage.replace(
                    self.job.sector.state.stage(self.job.planner()).to_string(),
                );
            })
            .crit()?;

        tracing::debug!("handling");

        self.planner.exec(&mut self.job)
    }
}
