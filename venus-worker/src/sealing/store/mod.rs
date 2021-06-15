use std::fs::{read, remove_dir_all};
use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::Result;
use serde::{Deserialize, Serialize};

use crate::metadb::rocks::RocksMeta;

const SUB_PATH_DATA: &str = "data";
const SUB_PATH_META: &str = ".meta";
const CONFIG_FILE_NAME: &str = "config.toml";

#[derive(Debug, Serialize, Deserialize)]
pub struct Config {
    pub max_retries: u32,
    pub enable_deal: bool,
    pub reserved_capacity: usize,

    #[serde(with = "humantime_serde")]
    pub seal_interval: Option<Duration>,

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

pub struct Store {
    pub location: PathBuf,
    pub data_path: PathBuf,

    pub config: Config,

    pub meta: RocksMeta,
}

impl Store {
    pub fn open<P: AsRef<Path>>(loc: P) -> Result<Self> {
        let content = read(loc.as_ref().join(CONFIG_FILE_NAME))?;
        let config: Config = toml::from_slice(&content)?;

        let db = RocksMeta::open(loc.as_ref().join(SUB_PATH_META))?;

        Ok(Store {
            location: loc.as_ref().to_owned(),
            data_path: loc.as_ref().join(SUB_PATH_DATA),
            config,
            meta: db,
        })
    }

    pub fn cleanup(&self) -> Result<()> {
        remove_dir_all(&self.data_path)?;
        Ok(())
    }
}
