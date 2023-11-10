//! config for damocles-worker

use std::collections::{HashMap, HashSet};
use std::fs::{self, File};
use std::io;
use std::net::SocketAddr;
use std::path::{Path, PathBuf};
use std::str;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use serde::{Deserialize, Serialize};

use crate::objstore::attached;
use crate::sealing::processor::external::config::Ext;

/// Default worker server port
pub const DEFAULT_WORKER_SERVER_PORT: u16 = 17890;
pub const DEFAULT_WORKER_SERVER_HOST: &str = "0.0.0.0";
/// The localhost addr
pub const LOCAL_HOST: &str = "127.0.0.1";
pub const DEFAULT_WORKER_PING_INTERVAL: Duration = Duration::from_secs(30);

pub const FILENAME_SECTORSTORE_JSON: &str = "sectorstore.json";

/// configurations for sealing sectors
#[derive(Debug, Clone, PartialEq, Eq)]
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

    /// max retry times for request task from sector manager
    pub request_task_max_retries: u32,
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
            request_task_max_retries: 3,
        }
    }
}

/// configurations for sealing sectors
#[derive(Debug, Default, Clone, Serialize, Deserialize)]
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

    /// max retry times for request task from sector manager
    pub request_task_max_retries: Option<u32>,
}

/// configuration for remote store
#[derive(Debug, Default, Serialize, Deserialize, Clone)]
pub struct Attached {
    pub name: Option<String>,
    /// store path, if we are using fs based store
    pub location: String,

    pub readonly: Option<bool>,
}

impl Attached {
    pub fn name(&self) -> &String {
        self.name.as_ref().unwrap_or(&self.location)
    }
}

/// configurations for local sealing store
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SealingThread {
    /// store location
    pub location: Option<String>,

    #[serde(flatten)]
    pub inner: SealingThreadInner,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SealingThreadInner {
    /// sealing plan
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

    /// section for unseal processor
    pub unseal: Option<Vec<Ext>>,

    /// section for window_post processor
    pub window_post: Option<Vec<Ext>>,
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

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SerdeDuration(#[serde(with = "humantime_serde")] pub Duration);

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct WorkerInstanceConfig {
    pub name: Option<String>,
    pub rpc_server: Option<RPCServer>,

    #[serde(default)]
    #[serde(with = "humantime_serde")]
    pub ping_interval: Option<Duration>,

    /// local pieces file directory, if set, worker will load the piece file from the local file
    /// otherwise it will load the remote piece file from damocles-manager
    pub local_pieces_dir: Option<PathBuf>, // For compatibility
    pub local_pieces_dirs: Option<Vec<PathBuf>>,

    /// Configure directories for scanning persistence store
    #[serde(default)]
    #[serde(alias = "ScanPersistStores")]
    pub scan_persist_stores: Vec<String>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct SectorManagerConfig {
    pub rpc_client: RPCClient,
    pub piece_token: Option<String>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct MetricsConfig {
    #[serde(default)]
    pub enable: bool,
    pub http_listen: Option<SocketAddr>,
}

/// global configuration
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Config {
    /// section for local config
    pub worker: Option<WorkerInstanceConfig>,

    /// section for metrics
    #[serde(default)]
    pub metrics: MetricsConfig,

    /// section for sector manager rpc
    pub sector_manager: SectorManagerConfig,

    /// section for common sealing
    #[serde(default)]
    pub sealing: SealingOptional,

    /// section for list of local sealing stores
    pub sealing_thread: Vec<SealingThread>,

    /// section for attached store
    pub attached: Option<Vec<Attached>>,

    /// section for processors
    #[serde(default)]
    pub processors: Processors,
}

impl Config {
    /// load config from the bytes
    pub fn from_bytes(bytes: &[u8]) -> Result<Self> {
        toml::from_str(str::from_utf8(bytes)?).context("deserialize config")
    }

    /// load from config file
    pub fn load<P: AsRef<Path>>(p: P) -> Result<Self> {
        let bytes = fs::read(p.as_ref()).with_context(|| {
            format!("read config file: {}", p.as_ref().display())
        })?;
        Self::from_bytes(&bytes)
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

        let addr = format!("{}:{}", host, port).parse().with_context(|| {
            format!("parse listen address with host: {}, port: {}", host, port)
        })?;
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
        let addr = format!("{}:{}", host, port).parse().with_context(|| {
            format!("parse connect address with host: {}, port: {}", host, port)
        })?;
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

    /// get worker local pieces dirs
    pub fn worker_local_pieces_dirs(&self) -> Vec<PathBuf> {
        self.worker
            .as_ref()
            .map(|worker| {
                match (&worker.local_pieces_dir, &worker.local_pieces_dirs) {
                    (_, Some(dirs)) => dirs.clone(),
                    (Some(dir), None) => {
                        vec![dir.clone()]
                    }
                    (None, None) => Vec::new(),
                }
            })
            .unwrap_or_default()
    }

    /// render the config content
    pub fn render(&self) -> Result<String> {
        use std::io::Write;

        let mut buf = Vec::new();
        writeln!(&mut buf, "{:#?}", self)?;
        Ok(String::from_utf8(buf)?)
    }

    /// attached returns all attached persist stores
    pub fn attached(&self) -> Result<Vec<Attached>> {
        let mut attached = self.attached.clone().unwrap_or(Vec::new());
        if let Some(worker) = &self.worker {
            if !worker.scan_persist_stores.is_empty() {
                attached.extend(
                    scan_persist_stores(&worker.scan_persist_stores)
                        .context("scan persist stores")?,
                );
            }
        }

        let mut unique_names = HashSet::new();
        for x in &attached {
            let name = x.name();
            if unique_names.contains(name) {
                return Err(anyhow!("duplicate persist name: {}", name));
            }
            unique_names.insert(name.clone());
        }
        Ok(attached)
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "PascalCase")]
struct SectorStoreJson {
    #[serde(rename = "ID")]
    pub id: Option<String>,
    pub name: Option<String>,
    pub read_only: Option<bool>,
}

pub fn scan_persist_stores(patterns: &[String]) -> Result<Vec<Attached>> {
    let mut attached = Vec::new();
    for pattern in patterns {
        for path in glob::glob(pattern)
            .with_context(|| format!("invalid glob pattern: {}", pattern))?
            .filter_map(Result::ok)
        {
            let cfg_path = path.join(FILENAME_SECTORSTORE_JSON);
            match fs::metadata(&cfg_path) {
                Ok(m) => {
                    if !m.is_file() {
                        continue;
                    }
                }
                Err(e) if e.kind() == io::ErrorKind::NotFound => {
                    continue;
                }
                Err(e) => {
                    return Err(anyhow!(
                        "open file {}: {:?}",
                        path.display(),
                        e
                    ));
                }
            }

            let f = File::open(&cfg_path).with_context(|| {
                format!("open file: {}", cfg_path.display())
            })?;
            let cfg: SectorStoreJson = serde_json::from_reader(f)
                .with_context(|| {
                    format!("deserialize json: {}", cfg_path.display())
                })?;

            let name = match (cfg.name, cfg.id) {
                (Some(name), _) if !name.is_empty() => Some(name),
                (_, Some(id)) if !id.is_empty() => Some(id),
                _ => None,
            };

            tracing::info!(
                "scanned persist store: {}, path: {}",
                name.as_deref().unwrap_or("None"),
                path.display()
            );

            attached.push(Attached {
                name,
                location: path.display().to_string(),
                readonly: cfg.read_only,
            })
        }
    }
    Ok(attached)
}

#[cfg(test)]
mod tests {
    use std::fs;

    use pretty_assertions::assert_eq;

    use super::FILENAME_SECTORSTORE_JSON;

    #[test]
    fn test_scan_persist_stores() {
        let tmpdir = tempfile::tempdir().unwrap();

        fs::create_dir(tmpdir.path().join("1")).unwrap();
        fs::create_dir(tmpdir.path().join("2")).unwrap();
        fs::create_dir(tmpdir.path().join("3")).unwrap();

        fs::write(
            tmpdir.path().join("1").join(FILENAME_SECTORSTORE_JSON),
            r#"{
            	"ID": "123",
            	"Name": "",
            	"Strict": false,
            	"ReadOnly": false,
            	"Weight": 0,
            	"AllowMiners": [1],
            	"DenyMiners": [2],
            	"PluginName": "2234234",
            	"Meta": {},
            	"CanSeal": true
            }"#,
        )
        .unwrap();

        fs::write(
            tmpdir.path().join("2").join(FILENAME_SECTORSTORE_JSON),
            r#"{
            	"Name": "456",
                "Meta": {},
                "Strict": true,
                "ReadOnly": false,
                "AllowMiners": [1],
                "DenyMiners": [2],
                "PluginName": "2234234",
                "CanSeal": true
            }"#,
        )
        .unwrap();

        let attached = super::scan_persist_stores(&[tmpdir
            .path()
            .join("*")
            .display()
            .to_string()])
        .unwrap();
        assert_eq!(attached.len(), 2);
        assert_eq!(attached[0].name, Some("123".to_string()));
        assert_eq!(attached[1].name, Some("456".to_string()));
    }
}
