use std::{fs::read_dir, path::Path};

use anyhow::Result;

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

    return Ok(size);
}
