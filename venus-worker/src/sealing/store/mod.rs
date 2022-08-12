//! definition of the sealing store

use std::collections::HashSet;
use std::convert::TryInto;
use std::fs::{create_dir_all, read_dir, remove_dir_all};
use std::path::{Path, PathBuf};

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use fil_types::ActorID;

use crate::infra::util::PlaceHolder;
use crate::logging::warn;
use crate::metadb::rocks::RocksMeta;
use crate::sealing::{
    hot_config::{result_flatten, HotConfig},
    worker::{Ctrl, Worker},
};
use crate::types::SealProof;

use crate::config::{Sealing, SealingOptional, SealingThread};

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

    fn hot_config_path(&self) -> PathBuf {
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

    /// storage usage plan
    pub plan: Option<String>,

    /// sub path for data dir
    pub data_path: PathBuf,

    /// embedded meta database for current store
    pub meta: RocksMeta,
    meta_path: PathBuf,

    /// allowed miners parsed from config
    pub allowed_miners: Option<Vec<ActorID>>,

    /// allowed proof types from config
    pub allowed_proof_types: Option<Vec<SealProof>>,

    hot_config: HotConfig<Config, SealingThread>,
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
    pub fn open(loc: PathBuf, config: Sealing, plan: Option<String>) -> Result<Self> {
        let location = Location(loc);

        let data_path = location.data_path();
        if !data_path.symlink_metadata().context("read file metadata")?.is_dir() {
            return Err(anyhow!("{:?} is not a dir", data_path));
        }

        let meta_path = location.meta_path();
        let meta = RocksMeta::open(&meta_path).with_context(|| format!("open metadb {:?}", meta_path))?;

        let default_config = Config { plan, sealing: config };
        let hot_config = HotConfig::new(default_config, merge_config, location.hot_config_path()).context("new HotConfig")?;
        let config = hot_config.config();

        let allowed_miners = config.sealing.allowed_miners.as_ref().cloned();
        let allowed_proof_types = config
            .sealing
            .allowed_sizes
            .as_deref()
            .map(Self::size_strings_to_proof_types)
            .transpose()?;

        let _holder = PlaceHolder::open(&location).context("open placeholder")?;

        Ok(Self {
            location,
            plan: config.plan.clone(),
            data_path,
            meta,
            meta_path,
            allowed_miners,
            allowed_proof_types,
            hot_config,
            _holder,
        })
    }

    /// Returns the sealing config
    pub fn config(&self) -> &Sealing {
        &self.hot_config.config().sealing
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

    /// Reload hot config when the content of hot config modified
    pub fn reload_if_needed(&mut self) -> Result<()> {
        result_flatten(self.hot_config.if_modified(|config| {
            self.plan = config.plan.clone();
            self.allowed_miners = config.sealing.allowed_miners.as_ref().cloned();
            self.allowed_proof_types = config
                .sealing
                .allowed_sizes
                .as_deref()
                .map(Self::size_strings_to_proof_types)
                .transpose()?;
            Ok(())
        }))
    }

    fn size_strings_to_proof_types(size_strings: &[String]) -> Result<Vec<SealProof>> {
        size_strings
            .iter()
            .map(|size_str| {
                Byte::from_str(size_str.as_str())
                    .with_context(|| format!("invalid size string {}", &size_str))
                    .and_then(|s| {
                        (s.get_bytes() as u64)
                            .try_into()
                            .with_context(|| format!("invalid SealProof from {}", &size_str))
                    })
            })
            .collect::<Result<Vec<_>>>()
    }
}

macro_rules! merge_fields {
    (SealingOptional, $common:expr, $cust:expr, $($field:ident,)+) => {
        SealingOptional {
            $(
                $field: $cust.as_ref().and_then(|c| c.$field.clone()).or_else(|| $common.$field.clone()),
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

fn customized_sealing_config(common: &SealingOptional, customized: Option<&SealingOptional>) -> Sealing {
    let default_cfg = Sealing::default();
    merge_fields! {
        common,
        customized,
        default_cfg,
        {
            allowed_miners,
            allowed_sizes,
            max_deals,
            min_deal_space,
        },
        {
            enable_deals,
            disable_cc,
            max_retries,
            seal_interval,
            recover_interval,
            rpc_polling_interval,
            ignore_proof_check,
        },
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct Config {
    plan: Option<String>,
    sealing: Sealing,
}

/// Merge hot config and default config
/// SealingThread::location cannot be override
fn merge_config(default_config: &Config, customized: SealingThread) -> Config {
    let default_sealing = default_config.sealing.clone();
    let SealingThread {
        plan: mut customized_plan,
        sealing: customized_sealingopt,
        ..
    } = customized;

    Config {
        plan: customized_plan.take().or_else(|| default_config.plan.clone()),
        sealing: match customized_sealingopt {
            Some(mut customized_sealingopt) => {
                merge_fields! {
                    Sealing,
                    default_sealing,
                    customized_sealingopt,
                    {
                        allowed_miners,
                        allowed_sizes,
                        max_deals,
                        min_deal_space,
                    },
                    {
                        enable_deals,
                        disable_cc,
                        max_retries,
                        seal_interval,
                        recover_interval,
                        rpc_polling_interval,
                        ignore_proof_check,
                    },
                }
            }
            None => default_sealing,
        },
    }
}

/// manages the sealing stores
#[derive(Default)]
pub struct StoreManager {
    stores: Vec<(PathBuf, Store)>,
}

impl StoreManager {
    /// loads specific
    pub fn load(list: &[SealingThread], common: &SealingOptional) -> Result<Self> {
        let mut stores = Vec::new();
        let mut path_set = HashSet::new();
        for scfg in list {
            let store_path = Path::new(&scfg.location)
                .canonicalize()
                .with_context(|| format!("canonicalize store path {}", scfg.location))?;

            if path_set.get(&store_path).is_some() {
                warn!(path = ?store_path, "store already loaded");
                continue;
            }

            let sealing_config = customized_sealing_config(common, scfg.sealing.as_ref());
            let store = Store::open(store_path.clone(), sealing_config, scfg.plan.as_ref().cloned())
                .with_context(|| format!("open store {:?}", store_path))?;
            path_set.insert(store_path.clone());
            stores.push((store_path, store));
        }

        Ok(StoreManager { stores })
    }

    /// build workers
    pub fn into_workers(self) -> Vec<(Worker, (usize, Ctrl))> {
        let mut workers = Vec::with_capacity(self.stores.len());
        for (idx, (_, store)) in self.stores.into_iter().enumerate() {
            let (w, c) = Worker::new(idx, store);
            workers.push((w, (idx, c)));
        }

        workers
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use pretty_assertions::assert_eq;

    use crate::config::{Sealing, SealingOptional, SealingThread};

    use super::{merge_config, Config};

    fn ms(millis: u64) -> Duration {
        Duration::from_millis(millis)
    }

    #[test]
    fn test_merge_config() {
        let cases = vec![
            (
                Config {
                    plan: Some("sealer".to_string()),
                    sealing: Default::default(),
                },
                SealingThread {
                    location: "ignore".to_string(),
                    plan: Some("sealer".to_string()),
                    sealing: None,
                },
                Config {
                    plan: Some("sealer".to_string()),
                    sealing: Default::default(),
                },
            ),
            (
                Config {
                    plan: Some("sealer".to_string()),
                    sealing: Sealing {
                        allowed_miners: None,
                        allowed_sizes: None,
                        enable_deals: true,
                        disable_cc: false,
                        max_deals: Some(100),
                        min_deal_space: None,
                        max_retries: 200,
                        seal_interval: ms(1000),
                        recover_interval: ms(1000),
                        rpc_polling_interval: ms(1000),
                        ignore_proof_check: true,
                    },
                },
                SealingThread {
                    location: "ignore".to_string(),
                    plan: Some("snapup".to_string()),
                    sealing: Some(SealingOptional {
                        allowed_miners: Some(vec![1, 2, 3]),
                        allowed_sizes: None,
                        enable_deals: Some(false),
                        disable_cc: Some(false),
                        max_deals: Some(100),
                        min_deal_space: None,
                        max_retries: Some(800),
                        seal_interval: Some(ms(2000)),
                        recover_interval: Some(ms(2000)),
                        rpc_polling_interval: Some(ms(1000)),
                        ignore_proof_check: None,
                    }),
                },
                Config {
                    plan: Some("snapup".to_string()),
                    sealing: Sealing {
                        allowed_miners: Some(vec![1, 2, 3]),
                        allowed_sizes: None,
                        enable_deals: false,
                        disable_cc: false,
                        max_deals: Some(100),
                        min_deal_space: None,
                        max_retries: 800,
                        seal_interval: ms(2000),
                        recover_interval: ms(2000),
                        rpc_polling_interval: ms(1000),
                        ignore_proof_check: true,
                    },
                },
            ),
        ];

        for (default_config, customized, expected) in cases {
            let actual = merge_config(&default_config, customized);
            assert_eq!(expected, actual);
        }
    }
}
