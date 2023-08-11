use std::marker::PhantomData;
use std::ops::Deref;
use std::sync::{Arc, RwLock};
use std::time::Instant;

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Receiver, Sender};
use vc_processors::core::{Processor, Task};

use crate::sealing::processor::LockedProcesssor;

use super::super::store::Location;

pub fn new_ctrl(loc: Option<Location>) -> (Ctrl, CtrlCtx) {
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
    RunningAt(Instant),
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

pub struct CtrlCtx {
    pub pause_rx: Receiver<()>,
    pub resume_rx: Receiver<Option<String>>,
    state: Arc<RwLock<CtrlState>>,
}

impl CtrlCtx {
    pub fn update_state<F: FnOnce(&mut CtrlState)>(&self, f: F) -> Result<()> {
        let mut ctrl_state = self.state.write().map_err(|e| anyhow!("rwlock posioned {:?}", e))?;

        f(&mut ctrl_state);
        drop(ctrl_state);
        Ok(())
    }

    pub fn wait<T, E>(&self, wait_fn: impl FnMut() -> Result<T, E>) -> Result<WaitGuard<T>, E> {
        WaitGuard::new(self.state.clone(), wait_fn)
    }
}

pub struct WaitGuard<T> {
    inner_guard: T,
    state: Arc<RwLock<CtrlState>>,
}

impl<T> Deref for WaitGuard<T> {
    type Target = T;

    fn deref(&self) -> &Self::Target {
        &self.inner_guard
    }
}

impl<T> WaitGuard<T> {
    fn new<E>(state: Arc<RwLock<CtrlState>>, mut wait_fn: impl FnMut() -> Result<T, E>) -> Result<Self, E> {
        state.write().unwrap().state = SealingThreadState::WaitAt(Instant::now());
        let inner_guard = wait_fn()?;
        state.write().unwrap().state = SealingThreadState::RunningAt(Instant::now());
        Ok(Self { inner_guard, state })
    }
}

impl<T> Drop for WaitGuard<T> {
    fn drop(&mut self) {
        self.state.write().unwrap().state = SealingThreadState::Idle;
    }
}

pub struct CtrlProcessor<T, LP, IP, G> {
    inner: LP,
    _pd: PhantomData<(T, IP, G)>,
}

impl<T, LP, IP, G> CtrlProcessor<T, LP, IP, G>
where
    T: Task,
    LP: LockedProcesssor<IP, G>,
    IP: Processor<T>,
{
    pub fn new(inner: LP) -> Self {
        Self { inner, _pd: PhantomData }
    }

    pub fn process(&self, ctx: &CtrlCtx, task: T) -> Result<<T as vc_processors::core::Task>::Output> {
        let guard = WaitGuard::new(ctx.state.clone(), move || self.inner.wait())?;
        guard.process(task)
    }
}

pub type CtrlProc<T, LP, G> = CtrlProcessor<T, LP, Box<dyn Processor<T>>, G>;
