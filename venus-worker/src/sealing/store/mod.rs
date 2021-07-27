//! definition of the sealing store

use std::collections::HashMap;
use std::fs::{create_dir_all, read_dir, remove_dir_all};
use std::path::{Path, PathBuf};

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Sender};

use crate::infra::util::PlaceHolder;
use crate::logging::{debug_field, warn};
use crate::metadb::rocks::RocksMeta;
use crate::sealing::worker::Worker;

use crate::config::{Sealing, SealingOptional, Store as StoreConfig};

pub mod util;

const SUB_PATH_DATA: &str = "data";
const SUB_PATH_META: &str = "meta";

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

    _holder: PlaceHolder,
}

impl Store {
    /// initialize the store at given location
    pub fn init<P: AsRef<Path>>(loc: P) -> Result<Location> {
        let location = Location(loc.as_ref().to_owned());
        create_dir_all(&location)?;

        let data_path = location.data_path();
        create_dir_all(&data_path)?;

        let _holder = PlaceHolder::init(loc)?;

        let meta_path = location.meta_path();
        let _ = RocksMeta::open(&meta_path)?;

        Ok(location)
    }

    /// opens the store at given location
    fn open(loc: PathBuf, config: Sealing) -> Result<Self> {
        let location = Location(loc);
        let data_path = location.data_path();
        if !data_path.symlink_metadata()?.is_dir() {
            return Err(anyhow!("{:?} is not a dir", data_path));
        }

        let _holder = PlaceHolder::open(&location)?;

        let meta_path = location.meta_path();
        let meta = RocksMeta::open(&meta_path)?;

        Ok(Store {
            location,
            data_path,
            config,
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
        let entries: Vec<_> = read_dir(&self.data_path)?.collect();
        if !entries.is_empty() {
            remove_dir_all(&self.data_path)?;
            create_dir_all(&self.data_path)?;
        }
        Ok(())
    }
}

macro_rules! merge_fields {
    (SealingOptional, $common:expr, $cust:expr, $($field:ident,)+) => {
        SealingOptional {
            $(
                $field: $cust.as_ref().and_then(|c| c.$field.clone()).or($common.$field.clone()),
            )+
        }
    };

    (Sealing, $def:expr, $merged:expr, {$($opt_field:ident,)*}, {$($field:ident,)*},) => {
        Sealing {
            $(
                $opt_field: $merged.$opt_field.take().or($def.$opt_field),
            )*

            $(
                $field: $merged.$field.take().unwrap_or($def.$field),
            )*
        }
    };

    ($common:expr, $cust:expr, $def:expr, {$($opt_field:ident,)*}, {$($field:ident,)*},) => {
        let mut merged = merge_fields! {
            SealingOptional,
            $common,
            $cust,
            $(
                $opt_field,
            )*
            $(
                $field,
            )*
        };

        merge_fields! {
            Sealing,
            $def,
            merged,
            {
                $(
                    $opt_field,
                )*
            },
            {
                $(
                    $field,
                )*
            },
        }
    };
}

fn customized_sealing_config(
    common: &SealingOptional,
    customized: Option<&SealingOptional>,
) -> Sealing {
    let default_cfg = Sealing::default();
    merge_fields! {
        common,
        customized,
        default_cfg,
        {
            allowed_miners,
            allowed_sizes,
        },
        {
            enable_deals,
            max_retries,
            seal_interval,
            recover_interval,
            rpc_polling_interval,
        },
    }
}

/// manages the sealing stores
#[derive(Default)]
pub struct StoreManager {
    stores: HashMap<PathBuf, Store>,
}

impl StoreManager {
    /// loads specific
    pub fn load(list: &[StoreConfig], common: &SealingOptional) -> Result<Self> {
        let mut stores = HashMap::new();
        for scfg in list {
            let store_path = Path::new(&scfg.location).canonicalize()?;

            if stores.get(&store_path).is_some() {
                warn!(path = debug_field(&store_path), "store already loaded");
                continue;
            }

            let sealing_config = customized_sealing_config(common, scfg.sealing.as_ref());
            let store = Store::open(store_path.clone(), sealing_config)?;
            stores.insert(store_path, store);
        }

        Ok(StoreManager { stores })
    }

    /// build workers
    pub fn into_workers(self) -> Vec<(Sender<()>, Worker)> {
        let mut workers = Vec::with_capacity(self.stores.len());
        for (_, store) in self.stores {
            let (resume_tx, resume_rx) = bounded(0);
            let worker = Worker::new(store, resume_rx);
            workers.push((resume_tx, worker));
        }

        workers
    }
}
