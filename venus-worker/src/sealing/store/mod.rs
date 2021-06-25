//! definition of the sealing store

use std::collections::BTreeMap;
use std::fs::{self, create_dir_all, read, read_dir, remove_dir_all};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::thread;

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Receiver};
use tracing::{error, warn};

use crate::infra::objstore::ObjectStore;
use crate::metadb::rocks::RocksMeta;
use crate::rpc::SealerRpcClient;
use crate::sealing::{resource::Pool, worker::Worker};

use super::config::Sealing;

pub mod util;

const SUB_PATH_DATA: &str = "data";
const SUB_PATH_META: &str = "meta";
const CONFIG_FILE_NAME: &str = "config.toml";

/// storage location
#[derive(Debug)]
pub struct Location(PathBuf);

impl AsRef<Path> for Location {
    fn as_ref(&self) -> &Path {
        &self.0
    }
}

impl Location {
    fn meta_path(&self) -> PathBuf {
        self.0.join(SUB_PATH_META)
    }

    fn data_path(&self) -> PathBuf {
        self.0.join(SUB_PATH_DATA)
    }

    fn config_file(&self) -> PathBuf {
        self.0.join(CONFIG_FILE_NAME)
    }
}

/// storage used in bytes
pub struct Usage {
    /// storage used for sealing data
    pub data: u64,

    /// storage used for metadb
    pub meta: u64,
}

/// a single sealing store
pub struct Store {
    /// storage location
    pub location: Location,

    /// sub path for data dir
    pub data_path: PathBuf,

    /// config for current store
    pub config: Sealing,

    /// embedded meta database for current store
    pub meta: RocksMeta,
    meta_path: PathBuf,
}

impl Store {
    /// initialize the store at given location
    pub fn init<P: AsRef<Path>>(loc: P, capacity: u64) -> Result<Self> {
        let location = Location(loc.as_ref().to_owned());
        create_dir_all(&location)?;

        let data_path = location.data_path();
        create_dir_all(&data_path)?;

        let mut config = Config::default();
        config.reserved_capacity = capacity;

        let cfg_content = toml::to_vec(&config)?;
        fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(location.config_file())
            .and_then(|mut f| f.write_all(&cfg_content))?;

        let meta_path = location.meta_path();
        let meta = RocksMeta::open(&meta_path)?;

        Ok(Store {
            location,
            data_path,
            config,
            meta,
            meta_path,
        })
    }

    /// opens the store at given location
    pub fn open<P: AsRef<Path>>(loc: P) -> Result<Self> {
        let location = loc.as_ref().canonicalize().map(|l| Location(l))?;
        let data_path = location.data_path();
        if !data_path.symlink_metadata()?.is_dir() {
            return Err(anyhow!("{:?} is not a dir", data_path));
        }

        let content = read(location.config_file())?;
        let config: Config = toml::from_slice(&content)?;

        let meta_path = location.meta_path();
        let meta = RocksMeta::open(&meta_path)?;

        Ok(Store {
            location,
            data_path,
            config,
            meta,
            meta_path,
        })
    }

    /// returns the disk usages inside the store
    pub fn usage(&self) -> Result<Usage> {
        let meta_used = util::disk_usage(&self.meta_path)?;
        let data_used = util::disk_usage(&self.data_path)?;
        Ok(Usage {
            meta: meta_used,
            data: data_used,
        })
    }

    /// cleanup cleans the store
    pub fn cleanup(&self) -> Result<()> {
        let entries: Vec<_> = read_dir(&self.data_path)?.collect();
        if !entries.is_empty() {
            remove_dir_all(&self.data_path)?;
            create_dir_all(&self.data_path)?;
        }
        Ok(())
    }
}

/// manages the sealing stores
#[derive(Default)]
pub struct StoreManager {
    stores: BTreeMap<String, Store>,
}

impl StoreManager {
    /// loads specific
    pub fn load(list: Vec<String>) -> Result<Self> {
        let mut stores = BTreeMap::new();
        for path in list {
            if stores.get(&path).is_some() {
                warn!(path = path.as_str(), "store already loaded");
                continue;
            }

            let store = Store::open(Path::new(&path))?;
            stores.insert(path, store);
        }

        Ok(StoreManager { stores })
    }

    /// start sealing loop
    pub fn start_sealing<O: ObjectStore + 'static>(
        self,
        done_rx: Receiver<()>,
        rpc: Arc<SealerRpcClient>,
        remote_store: Arc<O>,
        limit: Arc<Pool>,
    ) {
        let mut join_hdls = Vec::with_capacity(self.stores.len());
        let mut resume_txs = Vec::with_capacity(self.stores.len());
        for (_, store) in self.stores {
            let (resume_tx, resume_rx) = bounded(0);
            let mut worker = Worker::new(
                store,
                resume_rx,
                done_rx.clone(),
                rpc.clone(),
                remote_store.clone(),
                limit.clone(),
            );
            resume_txs.push(resume_tx);

            let hdl = thread::spawn(move || worker.start_seal());
            join_hdls.push(hdl);
        }

        for hdl in join_hdls {
            if let Err(e) = hdl
                .join()
                .map_err(|e| anyhow!("joined handler: {:?}", e))
                .and_then(|inner| inner)
            {
                error!("seal worker failure: {:?}", e);
            }
        }
    }
}
