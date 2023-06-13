use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::select;

use super::sealing_thread::Ctrl;

use crate::block_on;
use crate::logging::{debug, warn};
use crate::rpc::sealer::{WorkerInfo, WorkerInfoSummary};
use crate::watchdog::{Ctx, Module};

pub struct Ping {
    interval: Duration,
    ctrls: Arc<Vec<(usize, Ctrl)>>,
}

impl Ping {
    pub fn new(interval: Duration, ctrls: Arc<Vec<(usize, Ctrl)>>) -> Self {
        Ping { interval, ctrls }
    }

    fn summary(&self) -> Result<WorkerInfoSummary> {
        let mut sum = WorkerInfoSummary::default();

        for (idx, ctrl) in self.ctrls.as_ref().iter() {
            sum.threads += 1;
            ctrl.load_state(|cst| {
                if cst.job.id.is_none() {
                    sum.empty += 1;
                }

                if cst.job.last_error.is_some() {
                    sum.errors += 1;
                }

                if cst.paused_at.is_some() {
                    sum.paused += 1;
                }
            })
            .with_context(|| format!("load state from ctrl for sealing thread {}", idx))?;
        }

        Ok(sum)
    }
}

impl Module for Ping {
    fn should_wait(&self) -> bool {
        false
    }

    fn id(&self) -> String {
        "worker-ping".to_owned()
    }

    fn run(&mut self, ctx: Ctx) -> anyhow::Result<()> {
        let _rt_guard = ctx.global.rt.enter();
        loop {
            select! {
                recv(ctx.done) -> _ => {
                    return Ok(());
                }

                default(self.interval) => {
                }
            }

            let start = Instant::now();
            match self.summary().and_then(|sum| {
                let winfo = WorkerInfo {
                    name: ctx.instance.clone(),
                    version: (*crate::version::VERSION).clone(),
                    dest: ctx.dest.clone(),
                    summary: sum,
                };

                block_on(ctx.global.rpc.worker_ping(winfo)).map_err(|e| anyhow!("worker ping: {:?}", e))
            }) {
                Ok(()) => {
                    debug!(elapsed = ?start.elapsed(), "ping");
                }

                Err(e) => {
                    warn!(elapsed = ?start.elapsed(), "failed to ping: {:?}", e);
                }
            };
        }
    }
}
