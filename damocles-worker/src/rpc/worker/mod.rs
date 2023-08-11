use std::{fmt, path::PathBuf, time::Duration};

use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

/// {"type":"PausedAt","secs":123}
/// {"type":"Idle"}
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type", content = "secs")]
pub enum SealingThreadState {
    Idle,
    Paused(u64),
    Running(u64),
    Waiting(u64),
}

impl fmt::Display for SealingThreadState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use humantime::format_duration;

        match self {
            SealingThreadState::Idle => f.write_str("Idle"),
            SealingThreadState::Paused(x) => f.write_fmt(format_args!("Paused({})", format_duration(Duration::from_secs(*x)))),
            SealingThreadState::Running(x) => f.write_fmt(format_args!("Running({})", format_duration(Duration::from_secs(*x)))),
            SealingThreadState::Waiting(x) => f.write_fmt(format_args!("Waiting({})", format_duration(Duration::from_secs(*x)))),
        }
    }
}

/// information about each worker thread
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WorkerInfo {
    /// store location
    pub location: PathBuf,

    /// current plan of the worker
    pub plan: String,

    pub job_id: Option<String>,

    /// index for other control operations
    pub index: usize,

    pub thread_state: SealingThreadState,

    /// current job state of the worker
    pub job_state: String,

    /// current job stage of the worker
    pub job_stage: Option<String>,

    pub last_error: Option<String>,
}

#[rpc]
/// api defs
pub trait Worker {
    /// show all workers
    #[rpc(name = "VenusWorker.WorkerList")]
    fn worker_list(&self) -> Result<Vec<WorkerInfo>>;

    /// pause specific worker
    #[rpc(name = "VenusWorker.WorkerPause")]
    fn worker_pause(&self, index: usize) -> Result<bool>;

    /// resume specific worker, with given state, if any
    #[rpc(name = "VenusWorker.WorkerResume")]
    fn worker_resume(&self, index: usize, set_to: Option<String>) -> Result<bool>;

    /// set os environment
    #[rpc(name = "VenusWorker.WorkerSetEnv")]
    fn worker_set_env(&self, name: String, value: String) -> Result<()>;

    /// remove os environment
    #[rpc(name = "VenusWorker.WorkerRemoveEnv")]
    fn worker_remove_env(&self, name: String) -> Result<()>;

    /// get version of the worker
    #[rpc(name = "VenusWorker.WorkerVersion")]
    fn worker_version(&self) -> Result<String>;
}
