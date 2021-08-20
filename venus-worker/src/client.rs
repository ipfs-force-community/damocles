//! module for worker rpc client

use anyhow::{anyhow, Result};
use jsonrpc_core_client::transports::ws::{self, ConnectInfo};

use crate::config::Config;
use crate::rpc::worker;

pub use worker::WorkerClient;

/// returns a worker client based on the given config
pub fn connect(cfg: &Config) -> Result<WorkerClient> {
    let addr = cfg.worker_server_listen_addr()?;
    let endpoint = format!("ws://{}", addr);

    let connect_req = ConnectInfo {
        url: endpoint,
        headers: Default::default(),
    };

    let client = ws::connect(connect_req).map_err(|e| anyhow!("ws connect: {:?}", e))?;

    Ok(client)
}
