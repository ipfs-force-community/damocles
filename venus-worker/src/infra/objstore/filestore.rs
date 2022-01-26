//! ObjectStore implemented based on fs

use std::fs::{create_dir_all, File, OpenOptions};
use std::io::{copy, BufReader, Read, Seek, SeekFrom};
use std::path::{Path, PathBuf, MAIN_SEPARATOR};

use anyhow::{anyhow, Context, Result};

use super::{ObjResult, ObjectStore, Range};
use crate::{
    infra::util::PlaceHolder,
    logging::{debug_field, trace},
};

const LOG_TARGET: &str = "filestore";

/// FileStore
pub struct FileStore {
    sep: String,
    local_path: PathBuf,
    instance: String,
    _holder: PlaceHolder,
}

impl FileStore {
    /// init filestore, create a placeholder file in its root dir
    pub fn init<P: AsRef<Path>>(p: P) -> Result<()> {
        create_dir_all(p.as_ref())?;
        let _holder = PlaceHolder::init(p.as_ref())?;

        Ok(())
    }

    /// open the file store at given path
    pub fn open<P: AsRef<Path>>(p: P, ins: Option<String>) -> Result<Self> {
        let dir_path = p.as_ref().canonicalize().context("canonicalize dir path")?;
        if !dir_path
            .metadata()
            .context("read dir metadata")
            .map(|meta| meta.is_dir())?
        {
            return Err(anyhow!("base path of the file store should a dir"));
        };

        let _holder = PlaceHolder::open(&dir_path).context("open placeholder")?;

        let instance = match ins.or(dir_path.to_str().map(|s| s.to_owned())) {
            Some(i) => i,
            None => {
                return Err(anyhow!(
                    "dir path {:?} may contain invalid utf8 chars",
                    dir_path
                ))
            }
        };

        Ok(FileStore {
            sep: MAIN_SEPARATOR.to_string(),
            local_path: dir_path,
            instance,
            _holder,
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
        trace!(target: LOG_TARGET, res = debug_field(&res), "get full path");
        Ok(res)
    }
}

impl ObjectStore for FileStore {
    /// get should return a reader for the given path
    fn get(&self, path: &Path) -> ObjResult<Box<dyn Read>> {
        trace!(target: LOG_TARGET, path = debug_field(path), "get");

        let f = OpenOptions::new().read(true).open(self.path(path)?)?;
        let r: Box<dyn Read> = Box::new(f);
        Ok(r)
    }

    /// put an object
    fn put(&self, path: &Path, mut r: Box<dyn Read>) -> ObjResult<u64> {
        trace!(target: LOG_TARGET, path = debug_field(path), "put");

        let dst = self.path(path)?;

        if let Some(parent) = dst.parent() {
            if !parent.exists() {
                trace!(
                    target: LOG_TARGET,
                    parent = debug_field(parent),
                    "create parent dir"
                );

                create_dir_all(parent)?;
            }
        }

        let mut f = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(true)
            .open(dst)?;

        copy(&mut r, &mut f).map_err(From::from)
    }

    fn instance(&self) -> String {
        self.instance.clone()
    }

    /// get specified pieces
    fn get_chunks(
        &self,
        path: &Path,
        ranges: &[Range],
    ) -> ObjResult<Box<dyn Iterator<Item = ObjResult<Box<dyn Read>>>>> {
        trace!(
            target: LOG_TARGET,
            path = debug_field(path),
            pieces = ranges.len(),
            "get_chunks"
        );

        let f = OpenOptions::new().read(true).open(self.path(path)?)?;
        let iter: Box<dyn Iterator<Item = ObjResult<Box<dyn Read>>>> = Box::new(ChunkReader {
            f,
            ranges: ranges.to_owned(),
            cur: 0,
        });

        Ok(iter)
    }
}

struct ChunkReader {
    f: File,
    ranges: Vec<Range>,
    cur: usize,
}

impl Iterator for ChunkReader {
    type Item = ObjResult<Box<dyn Read>>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.cur >= self.ranges.len() {
            return None;
        }

        self.cur += 1;

        let cur_range = self.ranges[self.cur - 1];
        let (offset, size) = (cur_range.offset, cur_range.size);

        let cf = match self.f.try_clone() {
            Ok(cf) => cf,
            Err(e) => return Some(Err(e.into())),
        };

        if let Err(e) = self.f.seek(SeekFrom::Start(offset)) {
            return Some(Err(e.into()));
        };

        let r = Box::new(BufReader::new(cf).take(size));

        Some(Ok(r))
    }
}
