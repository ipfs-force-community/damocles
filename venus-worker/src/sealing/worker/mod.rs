use std::error::Error as StdError;
use std::sync::Arc;
use std::thread::sleep;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam_channel::{bounded, select, Receiver, Sender, TryRecvError};
use crossbeam_utils::atomic::AtomicCell;

use crate::logging::{debug_field, error, info, warn};
use crate::watchdog::{Ctx, Module};

use super::store::{Location, Store};

mod sealer;
use sealer::Sealer;

mod event;
use event::Event;

mod failure;
use failure::*;

mod sector;
use sector::*;

type HandleResult = Result<Event, Failure>;

#[derive(Debug, Clone, Copy)]
pub struct Interrupt;

impl std::fmt::Display for Interrupt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("interrupt")
    }
}

impl StdError for Interrupt {}

impl Interrupt {
    fn into_failure(self) -> Failure {
        Failure(Level::Permanent, self.into())
    }
}

fn new_ctrl(loc: Location) -> (Ctrl, CtrlCtx) {
    let (pause_tx, pause_rx) = bounded(1);
    let (resume_tx, resume_rx) = bounded(0);
    let paused = Arc::new(AtomicCell::new(false));
    let paused_at = Arc::new(AtomicCell::new(None));
    let sealing_state = Arc::new(AtomicCell::new(State::Empty));
    let last_sealing_error = Arc::new(AtomicCell::new(None));

    (
        Ctrl {
            location: loc,
            pause_tx,
            resume_tx,
            paused: paused.clone(),
            paused_at: paused_at.clone(),
            sealing_state: sealing_state.clone(),
            last_sealing_error: last_sealing_error.clone(),
        },
        CtrlCtx {
            pause_rx,
            resume_rx,
            paused,
            paused_at,
            sealing_state,
            last_sealing_error,
        },
    )
}

pub struct Ctrl {
    pub location: Location,
    pub pause_tx: Sender<()>,
    pub resume_tx: Sender<Option<State>>,
    pub paused: Arc<AtomicCell<bool>>,
    pub paused_at: Arc<AtomicCell<Option<Instant>>>,
    pub sealing_state: Arc<AtomicCell<State>>,
    pub last_sealing_error: Arc<AtomicCell<Option<String>>>,
}

pub struct CtrlCtx {
    pause_rx: Receiver<()>,
    resume_rx: Receiver<Option<State>>,
    paused: Arc<AtomicCell<bool>>,
    paused_at: Arc<AtomicCell<Option<Instant>>>,
    sealing_state: Arc<AtomicCell<State>>,
    last_sealing_error: Arc<AtomicCell<Option<String>>>,
}

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

    fn seal_one(&mut self, ctx: &Ctx, event: Option<Event>) -> Result<(), Failure> {
        let s = Sealer::build(ctx, &self.ctrl_ctx, &self.store)?;
        s.seal(event)
    }
}

impl Module for Worker {
    fn id(&self) -> String {
        format!("worker-{}", self.idx)
    }

    fn run(&mut self, ctx: Ctx) -> Result<()> {
        let mut wait_for_resume = false;
        let mut resume_event = None;
        let resume_loop_tick = Duration::from_secs(1800);

        'SEAL_LOOP: loop {
            if wait_for_resume {
                warn!("waiting for resume signal");

                select! {
                    recv(self.ctrl_ctx.resume_rx) -> resume_res => {
                        // resume sealing procedure with given SetState target
                        resume_event = resume_res.map(|s_opt| s_opt.map(|s| Event::SetState(s))).context("resume signal channel closed unexpectedly")?;

                        wait_for_resume = false;
                        self.ctrl_ctx.paused.store(false);
                        self.ctrl_ctx.paused_at.store(None);
                        self.ctrl_ctx.last_sealing_error.store(None);
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

            if let Err(failure) = self.seal_one(&ctx, resume_event.take()) {
                let is_interrupt = (failure.1).is::<Interrupt>();
                if !is_interrupt {
                    error!(failure = debug_field(&failure), "sealing failed");
                } else {
                    warn!("sealing interruptted");
                }

                match failure.0 {
                    Level::Temporary | Level::Permanent | Level::Critical => {
                        if failure.0 == Level::Temporary {
                            error!("temporary error should not be popagated to the top level");
                        };

                        wait_for_resume = true;
                        self.ctrl_ctx.paused.store(true);
                        self.ctrl_ctx.paused_at.store(Some(Instant::now()));
                        if !is_interrupt {
                            self.ctrl_ctx
                                .last_sealing_error
                                .store(Some(format!("{:?}", failure)));
                        }
                        continue 'SEAL_LOOP;
                    }

                    Level::Abort => {}
                };
            }

            info!(
                duration = debug_field(self.store.config.seal_interval),
                "wait before sealing"
            );

            self.ctrl_ctx.sealing_state.store(State::Empty);

            sleep(self.store.config.seal_interval);
        }
    }
}
