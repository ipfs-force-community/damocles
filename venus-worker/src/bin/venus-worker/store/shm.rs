use std::{
    fs::{remove_file, File, OpenOptions},
    io, mem,
    path::{Path, PathBuf},
};

use anyhow::{Context, Result};
use bytesize::ByteSize;
use time::OffsetDateTime;
use tracing::{debug, warn};

pub fn init_shm_files(numa_node_idx: u32, size: ByteSize, num: usize, shm_numa_dir_pattern: String) -> Result<Vec> {
    let mut dir = PathBuf::from("/dev/shm");
    dir.push(
        shm_numa_dir_pattern
            .trim_matches('/')
            .replacen("$NUMA_NODE_INDEX", &numa_node_idx.to_string(), 1),
    );

    let filename = size.to_string_as(true).replace(' ', "_");

    let now = OffsetDateTime::now_local()
        .unwrap_or_else(|_| OffsetDateTime::now_utc())
        .unix_timestamp();

    let paths = (0..num)
        .map(|i| dir.join(format!("{}_{}_{}", filename, now, i)))
        .collect::<Vec<_>>();

    let files = paths
        .iter()
        .map(|p| {
            let file = ShmFile(p);
            file.create_and_allocate(size.as_u64())?;
            Ok(file)
        })
        .collect::<Result<Vec<_>>>()?;
    // All files are created successfully, which means the initialization of
    // the shared memory files is successful, avoid destroying any file
    files.into_iter().for_each(mem::forget);
    Ok(paths)
}

struct ShmFile<'a>(&'a Path);

impl<'a> ShmFile<'a> {
    fn create_and_allocate(&self, size: u64) -> Result<()> {
        let file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(self.0)
            .context("create new shm file")?;
        allocate_file(&file, size).context("allocate file")
    }
}

impl<'a> Drop for ShmFile<'a> {
    fn drop(&mut self) {
        if let Err(e) = remove_file(self.0) {
            warn!(err=?e, "Unable to destroy file: '{}'", self.0.display());
        }
    }
}

fn allocate_file(file: &File, size: u64) -> io::Result<()> {
    use std::os::unix::io::AsRawFd;

    file.set_len(size)?;

    let fd = file.as_raw_fd();
    let size: libc::off_t = size.try_into().unwrap();

    if unsafe { libc::fallocate(fd, 0, 0, size) } == 0 {
        return Ok(());
    }
    handle_enospc("fallocate()")?;

    Ok(())
}

fn handle_enospc(s: &str) -> io::Result<()> {
    let err = io::Error::last_os_error();
    let errno = err.raw_os_error().unwrap_or(0);
    debug!("allocate_file: {} failed errno={}", s, errno);
    if errno == libc::ENOSPC {
        return Err(err.into());
    }
    Ok(())
}
