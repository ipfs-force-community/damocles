use std::{fmt, path::PathBuf, time::Duration};

use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

/// {"state":"Running","elapsed":100,"proc":"xx"}
/// {"state":"Paused","elapsed":100}
/// {"state":"Idle"}
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "state")]
pub enum SealingThreadState {
    Idle,
    Paused { elapsed: u64 },
    Running { elapsed: u64, proc: Option<String> },
    Waiting { elapsed: u64 },
}

impl fmt::Display for SealingThreadState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        use humantime::format_duration;

        match self {
            SealingThreadState::Idle => f.write_str("Idle"),
            SealingThreadState::Paused { elapsed } => {
                f.write_fmt(format_args!(
                    "Paused({})",
                    format_duration(Duration::from_secs(*elapsed))
                ))
            }
            SealingThreadState::Running { elapsed, proc } => {
                f.write_fmt(format_args!(
                    "Running({}) {}",
                    format_duration(Duration::from_secs(*elapsed)),
                    proc.as_deref().unwrap_or("")
                ))
            }
            SealingThreadState::Waiting { elapsed } => {
                f.write_fmt(format_args!(
                    "Waiting({})",
                    format_duration(Duration::from_secs(*elapsed))
                ))
            }
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
    fn worker_resume(
        &self,
        index: usize,
        set_to: Option<String>,
    ) -> Result<bool>;

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
