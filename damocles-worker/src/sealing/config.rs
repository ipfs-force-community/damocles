use std::{fs, io, ops::Deref, path::PathBuf, time::SystemTime};

use anyhow::{bail, Context, Result};
use byte_unit::Byte;
use fil_types::ActorID;
use serde::Deserialize;

use crate::{
    config::{Sealing, SealingOptional, SealingThreadInner},
    sealing::sealing_thread::default_plan,
    types::seal_types_from_u64,
    SealProof,
};

/// The config of the sealing thread
pub struct Config {
    /// allowed miners parsed from config
    pub allowed_miners: Vec<ActorID>,

    /// allowed proof types from config
    pub allowed_proof_types: Vec<SealProof>,

    hot_config: HotConfig<SealingWithPlan, SealingThreadInner>,
}

impl Config {
    pub(crate) fn new(
        config: Sealing,
        plan: Option<String>,
        hot_config_path: Option<impl Into<PathBuf>>,
    ) -> Result<Self> {
        let default_config = SealingWithPlan {
            plan,
            sealing: config,
        };

        let hot_config_path = match hot_config_path {
            Some(hot_config_path) => hot_config_path.into(),
            None => {
                // If hot_config_path is Option::None (for example, wdpost planner does not require hot config file),
                // it means there is no hot config file.
                // We point the hot config file to a non-existent file or a meaningless file (such as "/tmp/xxx")
                // to avoid loading the hot config file
                PathBuf::from("/tmp/non-existent-file")
            }
        };

        let hot_config =
            HotConfig::new(default_config, merge_config, hot_config_path)
                .context("new HotConfig")?;
        let config = hot_config.config();
        tracing::info!(config = ?config, "sealing thread config");
        let (allowed_miners, allowed_proof_types) =
            Self::extract_allowed(&config.sealing)?;

        Ok(Self {
            allowed_miners,
            allowed_proof_types,
            hot_config,
        })
    }

    /// Reload hot config when the content of hot config modified
    pub fn reload_if_needed(
        &mut self,
        f: impl FnOnce(&SealingWithPlan, &SealingWithPlan) -> Result<bool>,
    ) -> Result<()> {
        if self.hot_config.if_modified(f)? {
            let config = self.hot_config.config();
            tracing::info!(config = ?config, "sealing thread reload hot config");

            (self.allowed_miners, self.allowed_proof_types) =
                Self::extract_allowed(&config.sealing)?;
        }

        Ok(())
    }

    /// Returns `true` if the hot config modified.
    pub fn check_modified(&self) -> bool {
        self.hot_config.check_modified()
    }

    /// Returns the plan config item
    pub fn plan(&self) -> &str {
        self.hot_config
            .config()
            .plan
            .as_deref()
            .unwrap_or_else(|| default_plan())
    }

    fn extract_allowed(
        sealing: &Sealing,
    ) -> Result<(Vec<ActorID>, Vec<SealProof>)> {
        let allowed_miners: Vec<ActorID> =
            sealing.allowed_miners.iter().flatten().cloned().collect();
        let allowed_proof_types: Vec<_> = sealing
            .allowed_sizes
            .iter()
            .flatten()
            .map(|size_str| {
                Byte::from_str(size_str.as_str())
                    .with_context(|| {
                        format!("invalid size string {}", &size_str)
                    })
                    .and_then(|s| {
                        seal_types_from_u64(s.get_bytes() as u64).with_context(
                            || format!("invalid SealProof from {}", &size_str),
                        )
                    })
            })
            .collect::<Result<Vec<_>>>()?
            .concat();
        Ok((allowed_miners, allowed_proof_types))
    }
}

impl Deref for Config {
    type Target = Sealing;

    fn deref(&self) -> &Self::Target {
        &self.hot_config.config().sealing
    }
}

pub(crate) fn merge_sealing_fields(
    default_sealing: Sealing,
    mut customized: SealingOptional,
) -> Sealing {
    macro_rules! merge_fields {
        ($def:expr, $merged:expr, {$($opt_field:ident,)*}, {$($field:ident,)*},) => {
            Sealing {
                $(
                    $opt_field: $merged.$opt_field.take().or($def.$opt_field),
                )*

                $(
                    $field: $merged.$field.take().unwrap_or($def.$field),
                )*
            }
        };
    }

    merge_fields! {
        default_sealing,
        customized,
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
            request_task_max_retries,
            verify_after_pc2,
        },
    }
}

/// sealing config with plan
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SealingWithPlan {
    /// sealing plan
    pub plan: Option<String>,
    /// sealing config
    pub sealing: Sealing,
}

/// Merge hot config and default config
/// SealingThread::location cannot be override
fn merge_config(
    default_config: &SealingWithPlan,
    mut customized: SealingThreadInner,
) -> SealingWithPlan {
    let default_sealing = default_config.sealing.clone();
    SealingWithPlan {
        plan: customized
            .plan
            .take()
            .or_else(|| default_config.plan.clone()),
        sealing: match customized.sealing {
            Some(customized_sealingopt) => {
                merge_sealing_fields(default_sealing, customized_sealingopt)
            }
            None => default_sealing,
        },
    }
}

enum ConfigEvent {
    Unchanged,
    Created(SystemTime),
    Deleted,
    Modified(SystemTime),
}

/// HotConfig can load configuration updates without restarts
/// `T` is the type of the config we need.
/// `U` is the type of the hot config file content.
/// merge the `hot_config: U` to `default_config: T` by `merge_config_fn`
pub struct HotConfig<T, U> {
    current_config: T,
    new_config: Option<T>,
    default_config: T,
    merge_config_fn: fn(&T, U) -> T,

    path: PathBuf,
    last_modified: Option<SystemTime>,
}

impl<T, U> HotConfig<T, U>
where
    T: Clone,
    U: for<'a> Deserialize<'a>,
{
    /// Returns a new HotConfig
    ///
    /// The `path` argument is the hot config file path.
    pub fn new(
        default_config: T,
        merge_config_fn: fn(&T, U) -> T,
        path: impl Into<PathBuf>,
    ) -> anyhow::Result<Self> {
        let mut hot = Self {
            current_config: default_config.clone(),
            new_config: None,
            default_config,
            merge_config_fn,
            path: path.into(),
            last_modified: None,
        };
        hot.if_modified(|_, _| Ok(true))?;
        Ok(hot)
    }

    /// Call the given function `f` if the content of the hot config file is modified
    /// or the hot config file deleted compared to the last check.
    ///
    /// Apply the new config if the `f` function returns true.
    pub fn if_modified(
        &mut self,
        f: impl FnOnce(&T, &T) -> anyhow::Result<bool>,
    ) -> anyhow::Result<bool> {
        self.try_load()?;

        let should_apply = match &self.new_config {
            Some(new_config) => f(&self.current_config, new_config)?,
            None => false,
        };
        if should_apply {
            self.current_config = self.new_config.take().unwrap();
        }
        Ok(should_apply)
    }

    // Returns `true` if the hot config modified.
    pub fn check_modified(&self) -> bool {
        !matches!(
            self.check_modified_inner().unwrap_or_else(|e| {
                tracing::error!(err=?e, "check modified error");

                // if `check_modified_inner` reports an error, the configuration is considered unchanged.
                ConfigEvent::Unchanged
            }),
            ConfigEvent::Unchanged
        )
    }

    /// Returns current config
    pub fn config(&self) -> &T {
        &self.current_config
    }

    fn check_modified_inner(&self) -> anyhow::Result<ConfigEvent> {
        Ok(
            match (
                fs::metadata(&self.path).and_then(|m| m.modified()),
                self.last_modified,
            ) {
                (Ok(latest_modified), Some(last_modified)) => {
                    if latest_modified != last_modified {
                        // hot config file modified
                        ConfigEvent::Modified(latest_modified)
                    } else {
                        ConfigEvent::Unchanged
                    }
                }

                // hot config file created
                (Ok(latest_modified), None) => {
                    ConfigEvent::Created(latest_modified)
                }

                (Err(e), last_modified_opt) => {
                    if e.kind() == io::ErrorKind::NotFound {
                        match last_modified_opt {
                            // If the hot config file does not exist, but the self.last_modified is Option::Some,
                            // that means the hot config file is deleted.
                            Some(_) => ConfigEvent::Deleted,
                            None => ConfigEvent::Unchanged,
                        }
                    } else {
                        bail!("check modified for hot config. err: {:?}", e)
                    }
                }
            },
        )
    }

    fn try_load(&mut self) -> anyhow::Result<()> {
        match self.check_modified_inner()? {
            ConfigEvent::Created(latest_modified)
            | ConfigEvent::Modified(latest_modified) => {
                self.last_modified = Some(latest_modified);

                let content =
                    fs::read_to_string(&self.path).with_context(|| {
                        format!("read hot config: '{}'", self.path.display())
                    })?;
                let hot_config: U = toml::from_str(&content)
                    .context("deserializes toml file for hot config")?;
                self.new_config = Some((self.merge_config_fn)(
                    &self.default_config,
                    hot_config,
                ));
            }
            ConfigEvent::Deleted => {
                if self.last_modified.take().is_some() {
                    self.new_config = Some(self.default_config.clone());
                }
            }
            ConfigEvent::Unchanged => {
                // do nothing
            }
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::{
        fs,
        path::{Path, PathBuf},
        time::Duration,
    };

    use pretty_assertions::assert_eq;
    use serde::{Deserialize, Serialize};

    use crate::config::{Sealing, SealingOptional, SealingThreadInner};

    use super::{merge_config, HotConfig, SealingWithPlan};

    fn ms(millis: u64) -> Duration {
        Duration::from_millis(millis)
    }

    #[test]
    fn test_merge_config() {
        let cases = vec![
            (
                SealingWithPlan {
                    plan: Some("sealer".to_string()),
                    sealing: Default::default(),
                },
                SealingThreadInner {
                    plan: Some("sealer".to_string()),
                    sealing: None,
                },
                SealingWithPlan {
                    plan: Some("sealer".to_string()),
                    sealing: Default::default(),
                },
            ),
            (
                SealingWithPlan {
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
                        request_task_max_retries: 10,
                        verify_after_pc2: false,
                    },
                },
                SealingThreadInner {
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
                        request_task_max_retries: Some(11),
                        verify_after_pc2: Some(true),
                    }),
                },
                SealingWithPlan {
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
                        request_task_max_retries: 11,
                        verify_after_pc2: true,
                    },
                },
            ),
        ];

        for (default_config, customized, expected) in cases {
            let actual = merge_config(&default_config, customized);
            assert_eq!(expected, actual);
        }
    }

    #[derive(Serialize, Deserialize, Default, Eq, PartialEq, Debug, Clone)]
    struct TestConfig {
        foo: Option<String>,
        bar: Option<String>,
    }

    fn merge_test_config(x: &TestConfig, y: TestConfig) -> TestConfig {
        TestConfig {
            foo: y.foo.or_else(|| x.foo.as_ref().cloned()),
            bar: y.bar.or_else(|| x.bar.as_ref().cloned()),
        }
    }

    macro_rules! test_config {
        (v0) => {
            TestConfig {
                foo: Some("This is default config".to_string()),
                bar: Some("bar: v0".to_string()),
            }
        };
        ($version:ident) => {
            TestConfig {
                foo: None,
                bar: Some(format!("bar {}", stringify!($version))),
            }
        };
    }

    macro_rules! expect_config {
        ($hot:expr, $expect:expr) => {
            let modified = $hot
                .if_modified(|_, _| Ok(true))
                .expect("failed to check hot config file");
            let actual_config =
                if modified { Some($hot.config()) } else { None };
            assert_eq!($expect, actual_config);
        };
    }

    #[test]
    fn test_hot_config_if_modified() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = test_config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");

        write_toml_file(&hot_config_path, &test_config!(v1));
        let mut hot = HotConfig::new(
            default_config.clone(),
            merge_test_config,
            &hot_config_path,
        )
        .expect("failed to new HotConfig");
        assert_eq!(
            &merge_test_config(&default_config, test_config!(v1)),
            hot.config()
        );
        // The config should not change without modifying the hot config file
        expect_config!(hot, None);

        sleep_1s();
        write_toml_file(&hot_config_path, &test_config!(v2));
        expect_config!(
            hot,
            Some(&merge_test_config(&default_config, test_config!(v2)))
        );
        // The config should not change without modifying the hot config file
        expect_config!(hot, None);

        fs::remove_file(&hot_config_path)
            .expect("failed to remove hot config file");
        expect_config!(hot, Some(&default_config));
        expect_config!(hot, None);

        sleep_1s();
        write_toml_file(&hot_config_path, &test_config!(v3));
        expect_config!(
            hot,
            Some(&merge_test_config(&default_config, test_config!(v3)))
        );
        // The config should not change without modifying the hot config file
        expect_config!(hot, None);
    }

    #[test]
    fn test_hot_config_if_modified_when_no_hot_config_file() {
        let default_config = test_config!(v0);
        let mut hot = HotConfig::new(
            default_config.clone(),
            merge_test_config,
            PathBuf::from("/non_exist_file"),
        )
        .expect("Failed to new HotConfig");
        assert_eq!(&default_config, hot.config());
        expect_config!(hot, None);
    }

    #[test]
    fn test_hot_config_if_modified_when_given_false() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = test_config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");
        let mut hot = HotConfig::new(
            default_config.clone(),
            merge_test_config,
            &hot_config_path,
        )
        .expect("failed to new HotConfig");

        sleep_1s();
        write_toml_file(&hot_config_path, &test_config!(v1));
        hot.if_modified(|_, _| Ok(false)).unwrap();
        assert_eq!(&default_config, hot.config());
    }

    #[test]
    fn test_hot_config_check_modified() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = test_config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");
        let mut hot =
            HotConfig::new(default_config, merge_test_config, &hot_config_path)
                .expect("failed to new HotConfig");
        assert!(!hot.check_modified());
        write_toml_file(&hot_config_path, &test_config!(v1));
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());

        fs::remove_file(&hot_config_path)
            .expect("failed to remove hot config file");
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());

        write_toml_file(&hot_config_path, &test_config!(v1));
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());
    }

    fn write_toml_file(path: impl AsRef<Path>, config: &TestConfig) {
        fs::write(
            path,
            toml::to_string(&config).expect("failed to serialize config"),
        )
        .expect("failed to create toml file");
    }

    fn sleep_1s() {
        std::thread::sleep(std::time::Duration::from_secs(1));
    }
}
