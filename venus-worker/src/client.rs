//! module for worker rpc client

use anyhow::Result;
use async_std::task::block_on;

use crate::config::Config;
use crate::rpc::worker;
use crate::rpc::ws;

pub use worker::WorkerClient;

/// returns a worker client based on the given config
pub fn connect(cfg: &Config) -> Result<WorkerClient> {
    let addr = cfg.worker_server_listen_addr()?;
    let endpoint = format!("ws://{}", addr);

    let connect_req = ws::Request::builder().uri(endpoint).body(())?;
    let client = block_on(ws::connect(connect_req))?;

    Ok(client)
}
