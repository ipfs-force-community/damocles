use std::path::PathBuf;

use jsonrpc_core::Result;
use jsonrpc_derive::rpc;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct WorkerInfo {
    pub location: PathBuf,
    pub index: usize,
    pub paused: bool,
    pub state: String,
}

#[rpc]
pub trait Worker {
    #[rpc(name = "VenusWorker.ListWorkers")]
    fn list_workers(&self) -> Result<Vec<WorkerInfo>>;

    #[rpc(name = "VenusWorker.WorkerPause")]
    fn pause_worker(&self, index: usize) -> Result<bool>;

    #[rpc(name = "VenusWorker.WorkerResume")]
    fn resume_worker(&self, index: usize, set_to: Option<String>) -> Result<bool>;
}
