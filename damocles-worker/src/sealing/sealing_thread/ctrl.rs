use std::sync::{Arc, RwLock};
use std::time::Instant;

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Receiver, Sender};

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
    pub last_error: Option<String>,
}

#[derive(Default)]
pub struct CtrlState {
    pub paused_at: Option<Instant>,
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
        let ctrl_state = self.state.read().map_err(|e| anyhow!("rwlock posioned {:?}", e))?;

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
}
