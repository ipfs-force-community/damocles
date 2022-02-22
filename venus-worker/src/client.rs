//! module for worker rpc client

use anyhow::{anyhow, Result};
use jsonrpc_core_client::transports::http;

use crate::block_on;
use crate::config::Config;
use crate::rpc::worker;

pub use worker::WorkerClient;

/// returns a worker client based on the given config
pub fn connect(cfg: &Config) -> Result<WorkerClient> {
    let addr = cfg.worker_server_connect_addr()?;
    let endpoint = format!("http://{}", addr);

    let client = block_on(async move { http::connect(&endpoint).await })
        .map_err(|e| anyhow!("http connect: {:?}", e))?;

    Ok(client)
}
