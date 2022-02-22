//! module for worker rpc client

use anyhow::{anyhow, Context, Result};
use jsonrpc_core_client::transports::http;
use tokio::runtime::Builder;

use crate::config::Config;
use crate::rpc::worker;

pub use worker::WorkerClient;

/// returns a worker client based on the given config
pub fn connect(cfg: &Config) -> Result<WorkerClient> {
    let addr = cfg.worker_server_listen_addr()?;
    let endpoint = format!("http://{}", addr);

    let runtime = Builder::new_current_thread()
        .enable_all()
        .build()
        .context("construct tokio runtime")?;

    let client = runtime
        .block_on(async move { http::connect(&endpoint).await })
        .map_err(|e| anyhow!("http connect: {:?}", e))?;

    Ok(client)
}
