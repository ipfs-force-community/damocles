//! module for worker rpc client

use std::net::SocketAddr;

use anyhow::{anyhow, Result};
use jsonrpc_core_client::transports::http;

use crate::block_on;
use crate::rpc::worker;

pub use worker::WorkerClient;

/// returns a worker client based on the given config
pub fn connect(host: SocketAddr) -> Result<WorkerClient> {
    let endpoint = format!("http://{}", host);

    let client = block_on(async move { http::connect(&endpoint).await }).map_err(|e| anyhow!("http connect: {:?}", e))?;

    Ok(client)
}
