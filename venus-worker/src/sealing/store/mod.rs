use std::fs::remove_dir_all;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::Result;

use crate::metadb::MetaDB;

pub struct Config {
    pub max_retries: u32,
    pub enable_deal: bool,
    pub seal_interval: Option<Duration>,
    pub recover_interval: Option<Duration>,
}

pub struct Store<DB> {
    pub location: PathBuf,
    pub data_path: PathBuf,

    pub config: Config,

    pub meta: DB,
}

impl<DB: MetaDB> Store<DB> {}

impl<DB> Store<DB> {
    pub fn cleanup(&self) -> Result<()> {
        remove_dir_all(&self.data_path)?;
        Ok(())
    }
}
