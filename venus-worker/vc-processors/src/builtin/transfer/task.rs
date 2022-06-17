use std::collections::HashMap;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::core::Task;

pub const STAGE_NAME: &str = "data_transfer";

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferStoreInfo {
    pub name: String,
    pub uri: PathBuf,
    pub meta: HashMap<String, String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TranferItem {
    pub store_name: Option<String>,
    pub uri: PathBuf,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferOption {
    pub allow_link: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Transfer {
    pub src: TranferItem,
    pub dest: TranferItem,
    pub opt: Option<TransferOption>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferTask {
    pub stores: HashMap<String, TransferStoreInfo>,
    pub transfers: Vec<Transfer>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferFailure {
    pub transfer: Transfer,
    pub failure_msg: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TransferTaskOutput {
    pub failures: Vec<TransferFailure>,
}

impl Task for TransferTask {
    const STAGE: &'static str = STAGE_NAME;

    type Output = TransferTaskOutput;
}
