//! config for venus-worker

use std::collections::HashMap;
use std::fs::File;
use std::io::Read;
use std::net::SocketAddr;
use std::path::Path;
use std::time::Duration;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use toml::from_slice;

use crate::sealing::processor::external::config::Ext;

pub const DEFAULT_WORKER_SERVER_PORT: u16 = 17890;
pub const DEFAULT_WORKER_SERVER_HOST: &str = "0.0.0.0";
pub const LOCAL_HOST: &str = "127.0.0.1";

/// configurations for sealing sectors
#[derive(Debug, Clone)]
pub struct Sealing {
    /// specified miner actors
    pub allowed_miners: Option<Vec<u64>>,

    /// specified sector sizes
    pub allowed_sizes: Option<Vec<String>>,

    /// enable sealing sectors with deal pieces
    pub enable_deals: bool,

    /// max retry times for tempoary failed sector
    pub max_retries: u32,

    /// interval between sectors
    pub seal_interval: Duration,

    /// interval between retry attempts
    pub recover_interval: Duration,

    /// interval between polling requests
    pub rpc_polling_interval: Duration,

    /// ignore proof state check
    pub ignore_proof_check: bool,
}

impl Default for Sealing {
    fn default() -> Self {
        Sealing {
            allowed_miners: None,
            allowed_sizes: None,
            enable_deals: false,
            max_retries: 5,
            seal_interval: Duration::from_secs(30),
            recover_interval: Duration::from_secs(30),
            rpc_polling_interval: Duration::from_secs(30),
            ignore_proof_check: false,
        }
    }
}

/// configurations for sealing sectors
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SealingOptional {
    /// specified miner actors
    pub allowed_miners: Option<Vec<u64>>,

    /// specified sector sizes
    pub allowed_sizes: Option<Vec<String>>,

    /// enable sealing sectors with deal pieces
    pub enable_deals: Option<bool>,

    /// max retry times for tempoary failed sector
    pub max_retries: Option<u32>,

    /// interval between sectors
    #[serde(default)]
    #[serde(with = "humantime_serde")]
    pub seal_interval: Option<Duration>,

    /// interval between retry attempts
    #[serde(default)]
    #[serde(with = "humantime_serde")]
    pub recover_interval: Option<Duration>,

    /// interval between polling requests
    #[serde(default)]
    #[serde(with = "humantime_serde")]
    pub rpc_polling_interval: Option<Duration>,

    /// ignore proof state check
    pub ignore_proof_check: Option<bool>,
}

/// configuration for remote store
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Remote {
    pub name: Option<String>,
    /// store path, if we are using fs based store
    pub location: Option<String>,
}

/// configurations for local sealing store
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SealingThread {
    /// store location
    pub location: String,

    /// special sealing configuration
    pub sealing: Option<SealingOptional>,
}

/// configurations for rpc
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct RPCClient {
    /// jsonrpc endpoint
    pub addr: String,
    pub headers: Option<HashMap<String, String>>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct RPCServer {
    /// jsonrpc endpoint
    pub host: Option<String>,
    pub port: Option<u16>,
}

/// configurations for processors
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Processors {
    pub limit: Option<HashMap<String, usize>>,

    /// static tree_d paths for cc sectors
    pub static_tree_d: Option<HashMap<String, String>>,

    /// static staged file paths for cc sectors
    pub static_staged: Option<HashMap<String, String>>,

    /// static pieces file paths for cc sectors
    pub static_pieces: Option<HashMap<String, String>>,

    pub tree_d: Option<Vec<Ext>>,

    pub pc1: Option<Vec<Ext>>,

    /// section for pc2 processor
    pub pc2: Option<Vec<Ext>>,

    /// section for c2 processor
    pub c2: Option<Vec<Ext>>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct WorkerInstanceConfig {
    pub name: Option<String>,
    pub rpc_server: Option<RPCServer>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SectorManagerConfig {
    pub rpc_client: RPCClient,
    pub piece_token: Option<String>,
}

/// global configuration
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Config {
    /// section for local config
    pub worker: Option<WorkerInstanceConfig>,

    /// section for sector manager rpc
    pub sector_manager: SectorManagerConfig,

    /// section for common sealing
    pub sealing: SealingOptional,

    /// section for list of local sealing stores
    pub sealing_thread: Vec<SealingThread>,

    /// section for remote store
    pub remote_store: Remote,

    /// section for processors
    pub processors: Processors,
}

impl Config {
    /// load config from the reader
    pub fn from_reader<R: Read>(mut r: R) -> Result<Self> {
        let mut content = Vec::with_capacity(1 << 10);
        r.read_to_end(&mut content).context("read content")?;

        let cfg = from_slice(&content).context("deserialize config")?;

        Ok(cfg)
    }

    /// load from config file
    pub fn load<P: AsRef<Path>>(p: P) -> Result<Self> {
        let f = File::open(p).context("open file")?;
        Self::from_reader(f)
    }
}

impl Config {
    /// get listen addr for worker server
    pub fn worker_server_listen_addr(&self) -> Result<SocketAddr> {
        let host = self
            .worker
            .as_ref()
            .and_then(|w| w.rpc_server.as_ref())
            .and_then(|c| c.host.as_ref())
            .map(|s| s.as_str())
            .unwrap_or(DEFAULT_WORKER_SERVER_HOST);

        let port = self
            .worker
            .as_ref()
            .and_then(|w| w.rpc_server.as_ref())
            .and_then(|c| c.port.as_ref())
            .cloned()
            .unwrap_or(DEFAULT_WORKER_SERVER_PORT);

        let addr = format!("{}:{}", host, port)
            .parse()
            .with_context(|| format!("parse listen address with host: {}, port: {}", host, port))?;
        Ok(addr)
    }

    /// get connect addr for worker server
    pub fn worker_server_connect_addr(&self) -> Result<SocketAddr> {
        let host = self
            .worker
            .as_ref()
            .and_then(|w| w.rpc_server.as_ref())
            .and_then(|c| c.host.as_ref())
            .map(|s| s.as_str())
            .unwrap_or(LOCAL_HOST);

        let port = self
            .worker
            .as_ref()
            .and_then(|w| w.rpc_server.as_ref())
            .and_then(|c| c.port.as_ref())
            .cloned()
            .unwrap_or(DEFAULT_WORKER_SERVER_PORT);

        let addr = format!("{}:{}", host, port).parse().with_context(|| {
            format!("parse connect address with host: {}, port: {}", host, port)
        })?;
        Ok(addr)
    }
}
