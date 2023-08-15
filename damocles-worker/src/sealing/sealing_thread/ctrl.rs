use core::fmt;
use std::marker::PhantomData;
use std::ops::Deref;
use std::sync::{Arc, RwLock};
use std::time::Instant;

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Receiver, Sender};
use vc_processors::core::{Processor, Task};

use crate::limit::SealingLimit;
use crate::sealing::processor::LockProcessor;
use crate::sealing::resource::Token;

use super::super::store::Location;

pub fn new_ctrl(loc: Option<Location>, limit: Arc<SealingLimit>) -> (Ctrl, CtrlCtx) {
    let (pause_tx, pause_rx) = bounded(1);
    let (resume_tx, resume_rx) = bounded(0);
    let state = Arc::new(RwLock::new(Default::default()));

    (
        Ctrl {
            location: loc,
            pause_tx,
            resume_tx,
            state: state.clone(),
        },
        CtrlCtx {
            pause_rx,
            resume_rx,
            state,
            limit,
        },
    )
}

#[derive(Default)]
pub struct CtrlJobState {
    pub id: Option<String>,
    pub plan: String,
    pub state: Option<String>,
    pub stage: Option<String>,
    pub last_error: Option<String>,
}

#[derive(Debug, Clone)]
pub enum SealingThreadState {
    Idle,
    PausedAt(Instant),
    Running { at: Instant, proc: String },
    WaitAt(Instant),
}

impl Default for SealingThreadState {
    fn default() -> Self {
        SealingThreadState::Idle
    }
}

#[derive(Default)]
pub struct CtrlState {
    pub state: SealingThreadState,
    pub job: CtrlJobState,
}

pub struct Ctrl {
    pub location: Option<Location>,
    pub pause_tx: Sender<()>,
    pub resume_tx: Sender<Option<String>>,
    state: Arc<RwLock<CtrlState>>,
}

impl Ctrl {
    pub fn load_state<T, F: FnOnce(&CtrlState) -> T>(&self, f: F) -> Result<T> {
        let ctrl_state: std::sync::RwLockReadGuard<'_, CtrlState> = self.state.read().map_err(|e| anyhow!("rwlock posioned {:?}", e))?;

        let res = f(&ctrl_state);
        drop(ctrl_state);
        Ok(res)
    }
}

pub struct DisplayWrapper<T> {
    inner: T,
    display: &'static str,
}

impl<T> fmt::Display for DisplayWrapper<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.display)
    }
}

impl<T> Deref for DisplayWrapper<T> {
    type Target = T;

    fn deref(&self) -> &Self::Target {
        &self.inner
    }
}

pub struct CtrlCtx {
    pub pause_rx: Receiver<()>,
    pub resume_rx: Receiver<Option<String>>,
    limit: Arc<SealingLimit>,
    state: Arc<RwLock<CtrlState>>,
}

impl CtrlCtx {
    pub fn update_state<F: FnOnce(&mut CtrlState)>(&self, f: F) -> Result<()> {
        let mut ctrl_state = self.state.write().map_err(|e| anyhow!("rwlock posioned {:?}", e))?;

        f(&mut ctrl_state);
        drop(ctrl_state);
        Ok(())
    }

    pub fn wait(&self, stage: impl AsRef<str>) -> Result<WaitGuard<Token>> {
        self.state.write().unwrap().state = SealingThreadState::WaitAt(Instant::now());
        let inner = self.limit.acquire_stage_limit(stage)?;
        self.state.write().unwrap().state = SealingThreadState::Running {
            at: Instant::now(),
            proc: "prepare".to_string(),
        };
        Ok(WaitGuard {
            inner,
            state: self.state.clone(),
        })
    }
}

pub struct WaitGuard<T> {
    inner: T,
    state: Arc<RwLock<CtrlState>>,
}

impl<T> Drop for WaitGuard<T> {
    fn drop(&mut self) {
        match self.state.read().unwrap().state {
            SealingThreadState::Idle => {}
            _ => {
                self.state.write().unwrap().state = SealingThreadState::Idle;
            }
        }
    }
}

impl<T> std::ops::Deref for WaitGuard<T> {
    type Target = T;

    fn deref(&self) -> &Self::Target {
        &self.inner
    }
}

pub struct CtrlProcessor<T, P, LP> {
    inner: LP,
    _pd: PhantomData<(T, P)>,
}

impl<'a, T, P, LP> CtrlProcessor<T, P, LP>
where
    T: Task,
    P: Processor<T>,
    LP: LockProcessor + 'a,
    LP::Guard<'a>: Deref<Target = P>,
{
    pub fn new(inner: LP) -> Self {
        Self { inner, _pd: PhantomData }
    }

    pub fn process(&'a self, ctx: &CtrlCtx, task: T) -> Result<<T as vc_processors::core::Task>::Output> {
        ctx.state.write().unwrap().state = SealingThreadState::WaitAt(Instant::now());
        let guard = self.inner.lock()?;
        ctx.state.write().unwrap().state = SealingThreadState::Running {
            at: Instant::now(),
            proc: guard.name(),
        };
        guard.process(task)
    }
}
