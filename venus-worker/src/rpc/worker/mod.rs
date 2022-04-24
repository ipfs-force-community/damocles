use std::path::PathBuf;

use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

/// information about each worker thread
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WorkerInfo {
    /// store location
    pub location: PathBuf,

    pub sector_id: Option<String>,

    /// index for other control operations
    pub index: usize,

    /// if the worker is paused
    pub paused: bool,

    pub paused_elapsed: Option<u64>,

    /// current sealing state of the worker
    pub state: String,

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
}
