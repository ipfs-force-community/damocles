use std::fs::{self, create_dir_all, read, remove_dir_all};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::Result;
use serde::{Deserialize, Serialize};

use crate::metadb::rocks::RocksMeta;

const SUB_PATH_DATA: &str = "data";
const SUB_PATH_META: &str = "meta";
const CONFIG_FILE_NAME: &str = "config.toml";

#[derive(Debug, Serialize, Deserialize)]
/// sealing storage config
pub struct Config {
    /// max retry times for temporary failed sealing sectors
    pub max_retries: u32,

    /// enable sector with deal pieces
    pub enable_deal: bool,

    /// reserved storage capacity for current location
    pub reserved_capacity: u64,

    /// interval for sectors
    #[serde(with = "humantime_serde")]
    pub seal_interval: Option<Duration>,

    /// interval for recovering failed sectors
    #[serde(with = "humantime_serde")]
    pub recover_interval: Option<Duration>,
}

impl Default for Config {
    fn default() -> Self {
        Config {
            max_retries: 5,
            enable_deal: false,
            reserved_capacity: 0,
            seal_interval: Some(Duration::from_secs(30)),
            recover_interval: Some(Duration::from_secs(30)),
        }
    }
}

/// a single sealing store
pub struct Store {
    /// storage location
    pub location: PathBuf,

    /// sub path for data dir
    pub data_path: PathBuf,

    /// config for current store
    pub config: Config,

    /// embedded meta database for current store
    pub meta: RocksMeta,
}

impl Store {
    /// initialize the store at given location
    pub fn init<P: AsRef<Path>>(loc: P, capacity: u64) -> Result<Self> {
        create_dir_all(loc.as_ref())?;

        let data_path = loc.as_ref().join(SUB_PATH_DATA);
        create_dir_all(&data_path)?;

        let mut config = Config::default();
        config.reserved_capacity = capacity;

        let cfg_content = toml::to_vec(&config)?;
        fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(loc.as_ref().join(CONFIG_FILE_NAME))
            .and_then(|mut f| f.write_all(&cfg_content))?;

        let meta = RocksMeta::open(loc.as_ref().join(SUB_PATH_META))?;

        Ok(Store {
            location: loc.as_ref().to_owned(),
            data_path,
            config,
            meta,
        })
    }

    /// opens the store at given location
    pub fn open<P: AsRef<Path>>(loc: P) -> Result<Self> {
        let content = read(loc.as_ref().join(CONFIG_FILE_NAME))?;
        let config: Config = toml::from_slice(&content)?;

        let meta = RocksMeta::open(loc.as_ref().join(SUB_PATH_META))?;

        Ok(Store {
            location: loc.as_ref().to_owned(),
            data_path: loc.as_ref().join(SUB_PATH_DATA),
            config,
            meta,
        })
    }

    /// cleanup cleans the store
    pub fn cleanup(&self) -> Result<()> {
        remove_dir_all(&self.data_path)?;
        Ok(())
    }
}
