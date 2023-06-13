//! definition of the sealing store

use std::fs::{create_dir_all, read_dir, remove_dir_all};
use std::path::{Path, PathBuf};

use anyhow::{anyhow, Context, Result};

use crate::infra::util::PlaceHolder;
use crate::metadb::rocks::RocksMeta;

pub mod util;

const SUB_PATH_DATA: &str = "data";
const SUB_PATH_META: &str = "meta";
const SUB_PATH_HOT_CONFIG: &str = "config.toml";

/// storage location
#[derive(Debug, Clone)]
pub struct Location(PathBuf);

impl AsRef<Path> for Location {
    fn as_ref(&self) -> &Path {
        &self.0
    }
}

impl Location {
    /// Creates a new `Location` with a given PathBuf
    pub fn new(inner: PathBuf) -> Self {
        Self(inner)
    }

    /// clone inner PathBuf
    pub fn to_pathbuf(&self) -> PathBuf {
        self.0.clone()
    }

    fn meta_path(&self) -> PathBuf {
        self.0.join(SUB_PATH_META)
    }

    fn data_path(&self) -> PathBuf {
        self.0.join(SUB_PATH_DATA)
    }

    pub(crate) fn hot_config_path(&self) -> PathBuf {
        self.0.join(SUB_PATH_HOT_CONFIG)
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

    /// embedded meta database for current store
    pub meta: RocksMeta,
    meta_path: PathBuf,

    _holder: PlaceHolder,
}

impl Store {
    /// initialize the store at given location
    pub fn init<P: AsRef<Path>>(loc: P) -> Result<Location> {
        let location = Location(loc.as_ref().to_owned());
        create_dir_all(&location)?;

        let data_path = location.data_path();
        create_dir_all(data_path)?;

        let _holder = PlaceHolder::init(loc)?;

        let meta_path = location.meta_path();
        let _ = RocksMeta::open(meta_path)?;

        Ok(location)
    }

    /// opens the store at given location
    pub fn open(loc: PathBuf) -> Result<Self> {
        let location = Location(loc);

        let data_path = location.data_path();
        if !data_path.symlink_metadata().context("read file metadata")?.is_dir() {
            return Err(anyhow!("{:?} is not a dir", data_path));
        }

        let meta_path = location.meta_path();
        let meta = RocksMeta::open(&meta_path).with_context(|| format!("open metadb {:?}", meta_path))?;

        let _holder = PlaceHolder::open(&location).context("open placeholder")?;

        Ok(Self {
            location,
            data_path,
            meta,
            meta_path,
            _holder,
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
        if read_dir(&self.data_path)?.next().is_some() {
            remove_dir_all(&self.data_path)?;
            create_dir_all(&self.data_path)?;
        }
        Ok(())
    }
}
