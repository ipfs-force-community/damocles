//!  heplers for sealing store

use std::io::{BufRead, Cursor};
use std::{
    fs::{read, read_dir},
    path::Path,
};

use anyhow::Result;

/// get disk usage for given path
pub fn disk_usage<P: AsRef<Path>>(p: P) -> Result<u64> {
    let meta = p.as_ref().symlink_metadata()?;
    if meta.is_file() {
        return Ok(meta.len());
    }

    let mut size = 0;
    if meta.is_dir() {
        for entry in read_dir(p.as_ref())? {
            size += disk_usage(entry?.path())?;
        }
    }

    Ok(size)
}

/// load store paths from given file
pub fn load_store_list<P: AsRef<Path>>(p: P) -> Result<Vec<String>> {
    let reader = read(p).map(Cursor::new)?;
    let lines: Result<Vec<_>, _> = reader.lines().collect();
    Ok(lines?)
}
