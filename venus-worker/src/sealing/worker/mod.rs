use std::thread::sleep;

use anyhow::{Context, Result};
use crossbeam_channel::{select, Receiver, TryRecvError};

use crate::logging::{debug_field, error, info, warn};
use crate::watchdog::{Ctx, Module};

use super::{event::Event, failure::*, store::Store};

mod sealer;
use sealer::Sealer;

type HandleResult = Result<Event, Failure>;

pub struct Worker {
    store: Store,
    resume_rx: Receiver<()>,
}

impl Worker {
    pub fn new(s: Store, resume_rx: Receiver<()>) -> Self {
        Worker {
            store: s,
            resume_rx,
        }
    }

    fn seal_one(&mut self, ctx: &Ctx) -> Result<(), Failure> {
        let s = Sealer::build(ctx, &self.store)?;
        s.seal()
    }
}

impl Module for Worker {
    fn id(&self) -> String {
        format!("worker-{:?}", self.store.location.as_ref())
    }

    fn run(&mut self, ctx: Ctx) -> Result<()> {
        let mut wait_for_resume = false;
        'SEAL_LOOP: loop {
            if wait_for_resume {
                warn!("waiting for resume signal");

                select! {
                    recv(self.resume_rx) -> resume_res => {
                        resume_res.context("resume signal channel closed unexpectedly")?;
                    },

                    recv(ctx.done) -> _done_res => {
                        return Ok(())
                    },
                }
            }

            if ctx.done.try_recv() != Err(TryRecvError::Empty) {
                return Ok(());
            }

            if let Err(failure) = self.seal_one(&ctx) {
                error!(failure = debug_field(&failure), "sealing failed");
                match failure.0 {
                    Level::Temporary | Level::Permanent | Level::Critical => {
                        if failure.0 == Level::Temporary {
                            error!("temporary error should not be popagated to the top level");
                        };

                        wait_for_resume = true;
                        continue 'SEAL_LOOP;
                    }

                    Level::Abort => {}
                };
            }

            info!(
                duration = debug_field(self.store.config.seal_interval),
                "wait before sealing"
            );

            sleep(self.store.config.seal_interval);
        }
    }
}
