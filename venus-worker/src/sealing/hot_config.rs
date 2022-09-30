//! definition of the HotConfig

use std::fs;
use std::io;
use std::path::PathBuf;
use std::time::SystemTime;

use anyhow::{bail, Context};
use serde::Deserialize;
use tracing::error;

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
    pub fn new(default_config: T, merge_config_fn: fn(&T, U) -> T, path: impl Into<PathBuf>) -> anyhow::Result<Self> {
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
    pub fn if_modified(&mut self, f: impl FnOnce(&T, &T) -> anyhow::Result<bool>) -> anyhow::Result<bool> {
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
        match self.check_modified_inner().unwrap_or_else(|e| {
            error!(err=?e, "check modified error");

            // if `check_modified_inner` reports an error, the configuration is considered unchanged.
            ConfigEvent::Unchanged
        }) {
            ConfigEvent::Unchanged => false,
            _ => true,
        }
    }

    /// Returns current config
    pub fn config(&self) -> &T {
        &self.current_config
    }

    fn check_modified_inner(&self) -> anyhow::Result<ConfigEvent> {
        Ok(match (fs::metadata(&self.path).and_then(|m| m.modified()), self.last_modified) {
            (Ok(latest_modified), Some(last_modified)) => {
                if latest_modified != last_modified {
                    // hot config file modified
                    ConfigEvent::Modified(latest_modified)
                } else {
                    ConfigEvent::Unchanged
                }
            }

            // hot config file created
            (Ok(latest_modified), None) => ConfigEvent::Created(latest_modified),

            (Err(e), last_modified_opt) => {
                if e.kind() == io::ErrorKind::NotFound {
                    match last_modified_opt {
                        // If the hot config file does not exist, but the self.last_modified is some,
                        // that means the hot config file is deleted.
                        Some(_) => ConfigEvent::Deleted,
                        None => ConfigEvent::Unchanged,
                    }
                } else {
                    bail!("check modified for hot config. err: {:?}", e)
                }
            }
        })
    }

    fn try_load(&mut self) -> anyhow::Result<()> {
        match self.check_modified_inner()? {
            ConfigEvent::Created(latest_modified) | ConfigEvent::Modified(latest_modified) => {
                self.last_modified = Some(latest_modified);

                let content = fs::read_to_string(&self.path).with_context(|| format!("read hot config: '{}'", self.path.display()))?;
                let hot_config: U = toml::from_str(&content).context("deserializes toml file for hot config")?;
                self.new_config = Some((self.merge_config_fn)(&self.default_config, hot_config));
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
    use std::path::Path;
    use std::{fs, path::PathBuf};

    use pretty_assertions::assert_eq;
    use serde::{Deserialize, Serialize};

    use super::HotConfig;

    #[derive(Serialize, Deserialize, Default, Eq, PartialEq, Debug, Clone)]
    struct Config {
        foo: Option<String>,
        bar: Option<String>,
    }

    fn merge_config(x: &Config, y: Config) -> Config {
        Config {
            foo: y.foo.or_else(|| x.foo.as_ref().cloned()),
            bar: y.bar.or_else(|| x.bar.as_ref().cloned()),
        }
    }

    macro_rules! config {
        (v0) => {
            Config {
                foo: Some("This is default config".to_string()),
                bar: Some("bar: v0".to_string()),
            }
        };
        ($version:ident) => {
            Config {
                foo: None,
                bar: Some(format!("bar {}", stringify!($version))),
            }
        };
    }

    macro_rules! expect_config {
        ($hot:expr, $expect:expr) => {
            let modified = $hot.if_modified(|_, _| Ok(true)).expect("failed to check hot config file");
            let actual_config = if modified { Some($hot.config()) } else { None };
            assert_eq!($expect, actual_config);
        };
    }

    #[test]
    fn test_if_modified() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");

        write_toml_file(&hot_config_path, &config!(v1));
        let mut hot = HotConfig::new(default_config.clone(), merge_config, &hot_config_path).expect("failed to new HotConfig");
        assert_eq!(&merge_config(&default_config, config!(v1)), hot.config());
        // The config should not change without modifying the hot config file
        expect_config!(&mut hot, None);

        sleep_1s();
        write_toml_file(&hot_config_path, &config!(v2));
        expect_config!(&mut hot, Some(&merge_config(&default_config, config!(v2))));
        // The config should not change without modifying the hot config file
        expect_config!(&mut hot, None);

        fs::remove_file(&hot_config_path).expect("failed to remove hot config file");
        expect_config!(&mut hot, Some(&default_config));
        expect_config!(&mut hot, None);

        sleep_1s();
        write_toml_file(&hot_config_path, &config!(v3));
        expect_config!(&mut hot, Some(&merge_config(&default_config, config!(v3))));
        // The config should not change without modifying the hot config file
        expect_config!(&mut hot, None);
    }

    #[test]
    fn test_if_modified_when_no_hot_config_file() {
        let default_config = config!(v0);
        let mut hot =
            HotConfig::new(default_config.clone(), merge_config, PathBuf::from("/non_exist_file")).expect("Failed to new HotConfig");
        assert_eq!(&default_config, hot.config());
        expect_config!(&mut hot, None);
    }

    #[test]
    fn test_if_modified_when_given_false() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");
        let mut hot = HotConfig::new(default_config.clone(), merge_config, &hot_config_path).expect("failed to new HotConfig");

        sleep_1s();
        write_toml_file(&hot_config_path, &config!(v1));
        hot.if_modified(|_, _| Ok(false)).unwrap();
        assert_eq!(&default_config, hot.config());
    }

    #[test]
    fn test_check_modified() {
        let tempdir = tempfile::tempdir().expect("failed to create temp dir");
        let default_config = config!(v0);
        let hot_config_path = tempdir.path().join("hot.toml");
        let mut hot = HotConfig::new(default_config.clone(), merge_config, &hot_config_path).expect("failed to new HotConfig");
        assert!(!hot.check_modified());
        write_toml_file(&hot_config_path, &config!(v1));
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());

        fs::remove_file(&hot_config_path).expect("failed to remove hot config file");
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());

        write_toml_file(&hot_config_path, &config!(v1));
        assert!(hot.check_modified());
        assert!(hot.check_modified());
        hot.if_modified(|_, _| Ok(true)).unwrap();
        assert!(!hot.check_modified());
    }

    fn write_toml_file(path: impl AsRef<Path>, config: &Config) {
        fs::write(path, toml::to_string(&config).expect("failed to serialize config")).expect("failed to create toml file");
    }

    fn sleep_1s() {
        std::thread::sleep(std::time::Duration::from_secs(1));
    }
}
