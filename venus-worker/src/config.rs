//! config for venus-worker

use std::collections::HashMap;
use std::fs::File;
use std::io::Read;
use std::net::SocketAddr;
use std::path::Path;
use std::time::Duration;

use anyhow::{Context, Result};
use byte_unit::Byte;
use serde::{Deserialize, Serialize};
use toml::from_slice;

use crate::sealing::processor::external::config::Ext;

pub const DEFAULT_WORKER_SERVER_PORT: u16 = 17890;
pub const DEFAULT_WORKER_SERVER_HOST: &str = "0.0.0.0";
pub const LOCAL_HOST: &str = "127.0.0.1";
pub const DEFAULT_WORKER_PING_INTERVAL: Duration = Duration::from_secs(180);

/// configurations for sealing sectors
#[derive(Debug, Clone)]
pub struct Sealing {
    /// specified miner actors
    pub allowed_miners: Option<Vec<u64>>,

    /// specified sector sizes
    pub allowed_sizes: Option<Vec<String>>,

    /// enable sealing sectors with deal pieces
    pub enable_deals: bool,

    /// disable cc sectors when deals enabled
    pub disable_cc: bool,

    /// max limit of deals count inside one sector
    pub max_deals: Option<usize>,

    /// min used space for deals inside one sector
    pub min_deal_space: Option<Byte>,

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
            disable_cc: false,
            max_deals: None,
            min_deal_space: None,
            max_retries: 5,
            seal_interval: Duration::from_secs(30),
            recover_interval: Duration::from_secs(60),
            rpc_polling_interval: Duration::from_secs(180),
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

    /// disable cc sectors when deals enabled
    pub disable_cc: Option<bool>,

    /// max limit of deals count inside one sector
    pub max_deals: Option<usize>,

    /// min used space for deals inside one sector
    pub min_deal_space: Option<Byte>,

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
pub struct Attached {
    pub name: Option<String>,
    /// store path, if we are using fs based store
    pub location: String,

    pub readonly: Option<bool>,
}

/// configurations for local sealing store
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SealingThread {
    /// store location
    pub location: String,

    pub plan: Option<String>,

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
    // For compatibility, it is equivalent to `Limit.concurrent`
    pub limit: Option<HashMap<String, usize>>,

    #[serde(default)]
    pub limitation: Limit,

    pub ext_locks: Option<HashMap<String, usize>>,

    /// static tree_d paths for cc sectors
    pub static_tree_d: Option<HashMap<String, String>>,

    /// section for add_pieces processor
    pub add_pieces: Option<Vec<Ext>>,

    /// section for tree_d processor
    pub tree_d: Option<Vec<Ext>>,

    /// section for pc1 processor
    pub pc1: Option<Vec<Ext>>,

    /// section for pc2 processor
    pub pc2: Option<Vec<Ext>>,

    /// section for c2 processor
    pub c2: Option<Vec<Ext>>,

    /// section for snap_encode processor
    pub snap_encode: Option<Vec<Ext>>,

    /// section for snap_prove processor
    pub snap_prove: Option<Vec<Ext>>,

    /// section for transfer processor
    pub transfer: Option<Vec<Ext>>,
}

impl Processors {
    pub fn limitation_concurrent(&self) -> &Option<HashMap<String, usize>> {
        match (&self.limit, &self.limitation.concurrent) {
            (_, None) => &self.limit,
            (_, Some(_)) => &self.limitation.concurrent,
        }
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Limit {
    pub concurrent: Option<HashMap<String, usize>>,
    pub staggered: Option<HashMap<String, SerdeDuration>>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SerdeDuration(#[serde(with = "humantime_serde")] pub Duration);

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct WorkerInstanceConfig {
    pub name: Option<String>,
    pub rpc_server: Option<RPCServer>,

    #[serde(default)]
    #[serde(with = "humantime_serde")]
    pub ping_interval: Option<Duration>,
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

    /// section for remote store, deprecated
    pub remote_store: Option<Attached>,

    /// section for attached store
    pub attached: Option<Vec<Attached>>,

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

        let port = self.worker_server_listen_port();

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

        let port = self.worker_server_listen_port();
        let addr = format!("{}:{}", host, port)
            .parse()
            .with_context(|| format!("parse connect address with host: {}, port: {}", host, port))?;
        Ok(addr)
    }

    /// get listen port for worker server
    pub fn worker_server_listen_port(&self) -> u16 {
        self.worker
            .as_ref()
            .and_then(|w| w.rpc_server.as_ref())
            .and_then(|c| c.port.as_ref())
            .cloned()
            .unwrap_or(DEFAULT_WORKER_SERVER_PORT)
    }

    /// get worker ping interval
    pub fn worker_ping_interval(&self) -> Duration {
        self.worker
            .as_ref()
            .and_then(|w| w.ping_interval.as_ref())
            .cloned()
            .unwrap_or(DEFAULT_WORKER_PING_INTERVAL)
    }
}
