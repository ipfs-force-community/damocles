//! utilities for infra

use std::fs::{create_dir_all, File, OpenOptions};
use std::path::Path;

use anyhow::{anyhow, Result};

const PLACEHOLDER_NAME: &str = ".holder";

/// PlaceHolder constructor
pub struct PlaceHolder {
    _f: File,
}

impl PlaceHolder {
    /// init placeholder in the given dir
    pub fn init<P: AsRef<Path>>(dir: P) -> Result<Self> {
        create_dir_all(dir.as_ref())?;
        OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(dir.as_ref().join(PLACEHOLDER_NAME))
            .map(|f| PlaceHolder { _f: f })
            .map_err(|e| e.into())
    }

    /// open a placeholder file in read-only mode
    pub fn open<P: AsRef<Path>>(dir: P) -> Result<Self> {
        let f = OpenOptions::new()
            .read(true)
            .open(dir.as_ref().join(PLACEHOLDER_NAME))?;

        let meta = f.metadata()?;
        if !meta.is_file() {
            return Err(anyhow!("placeholder should be a file"));
        }

        Ok(PlaceHolder { _f: f })
    }
}
