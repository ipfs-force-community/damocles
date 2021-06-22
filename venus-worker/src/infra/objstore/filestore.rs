//! ObjectStore implemented based on fs

use std::fs::{File, OpenOptions};
use std::io::{copy, BufReader, Read, Seek, SeekFrom};
use std::path::{Path, PathBuf, MAIN_SEPARATOR};

use anyhow::{anyhow, Result};

use super::{ObjResult, ObjectStore, Range};

/// FileStore
pub struct FileStore {
    sep: String,
    path: PathBuf,
}

impl FileStore {
    /// open the file store at given path
    pub fn open<P: AsRef<Path>>(p: P) -> Result<Self> {
        let dir_path = p.as_ref();
        if !dir_path.metadata().map(|meta| meta.is_dir())? {
            return Err(anyhow!("base path of the file store should a dir"));
        };

        Ok(FileStore {
            sep: MAIN_SEPARATOR.to_string(),
            path: dir_path.to_owned(),
        })
    }

    fn path<P: AsRef<Path>>(&self, sub: P) -> ObjResult<PathBuf> {
        let p = sub.as_ref();
        if p.starts_with(".") {
            return Err(anyhow!("sub path starts with dot").into());
        }

        if p.starts_with(&self.sep) {
            return Err(anyhow!("sub path starts with separator").into());
        }

        Ok(self.path.join(sub))
    }
}

impl ObjectStore for FileStore {
    /// get should return a reader for the given path
    fn get<P: AsRef<Path>>(&self, path: P) -> ObjResult<Box<dyn Read>> {
        let f = OpenOptions::new().read(true).open(self.path(path)?)?;
        let r: Box<dyn Read> = Box::new(f);
        Ok(r)
    }

    /// put an object
    fn put<P: AsRef<Path>, R: Read>(&self, path: P, mut r: R) -> ObjResult<u64> {
        let mut f = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(true)
            .open(self.path(path)?)?;

        copy(&mut r, &mut f).map_err(From::from)
    }

    /// get specified pieces
    fn get_chunks<P: AsRef<Path>>(
        &self,
        path: P,
        ranges: &[Range],
    ) -> ObjResult<Box<dyn Iterator<Item = ObjResult<Box<dyn Read>>>>> {
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
