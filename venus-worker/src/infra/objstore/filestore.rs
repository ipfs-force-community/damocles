//! ObjectStore implemented based on fs

use std::fs::create_dir_all;
use std::path::{Path, PathBuf, MAIN_SEPARATOR};

use anyhow::{anyhow, Context, Result};

use super::{ObjResult, ObjectStore};
use crate::logging::trace;

const LOG_TARGET: &str = "filestore";

/// FileStore
pub struct FileStore {
    sep: String,
    local_path: PathBuf,
    instance: String,
    readonly: bool,
}

impl FileStore {
    /// init filestore, create a placeholder file in its root dir
    pub fn init<P: AsRef<Path>>(p: P) -> Result<()> {
        create_dir_all(p.as_ref())?;

        Ok(())
    }

    /// open the file store at given path
    pub fn open<P: AsRef<Path>>(p: P, ins: Option<String>, readonly: bool) -> Result<Self> {
        let dir_path = p.as_ref().canonicalize().context("canonicalize dir path")?;
        if !dir_path.metadata().context("read dir metadata").map(|meta| meta.is_dir())? {
            return Err(anyhow!("base path of the file store should a dir"));
        };

        let instance = match ins.or_else(|| dir_path.to_str().map(|s| s.to_owned())) {
            Some(i) => i,
            None => return Err(anyhow!("dir path {:?} may contain invalid utf8 chars", dir_path)),
        };

        Ok(FileStore {
            sep: MAIN_SEPARATOR.to_string(),
            local_path: dir_path,
            instance,
            readonly,
        })
    }

    fn path<P: AsRef<Path>>(&self, sub: P) -> ObjResult<PathBuf> {
        let mut p = sub.as_ref();
        if p.starts_with(".") {
            return Err(anyhow!("sub path starts with dot").into());
        }

        // try to strip the first any only the first sep
        if let Ok(strip) = p.strip_prefix(&self.sep) {
            p = strip;
        }

        if p.starts_with(&self.sep) {
            return Err(anyhow!("sub path starts with separator").into());
        }

        let res = self.local_path.join(sub);
        trace!(target: LOG_TARGET, ?res, "get full path");
        Ok(res)
    }
}

impl ObjectStore for FileStore {
    fn instance(&self) -> String {
        self.instance.clone()
    }

    fn uri(&self, rel: &Path) -> ObjResult<PathBuf> {
        self.path(rel)
    }

    fn readonly(&self) -> bool {
        self.readonly
    }
}
