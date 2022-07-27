use std::{
    fs::{self, remove_file, File, OpenOptions},
    io, mem,
    path::{Path, PathBuf},
};

use anyhow::{anyhow, Context, Result};
use bytesize::ByteSize;
use tracing::warn;
use vc_processors::sys::numa::Numa;

pub fn create_hugepage_mem_files(numa_node_idx: u32, size: ByteSize, count: usize, path: impl AsRef<Path>) -> Result<Vec<PathBuf>> {
    // bind NUMA node
    let numa = Numa::new().map_err(|_| anyhow!("NUMA not available"))?;
    numa.bind(numa_node_idx)
        .map_err(|_| anyhow!("invalid NUMA node: {}", numa_node_idx))?;

    let path = path.as_ref();
    fs::create_dir_all(&path).with_context(|| format!("create hugepage memory files directory: '{}'", path.display()))?;

    let filename = size.to_string_as(true).replace(' ', "_");

    let paths = (0..count).map(|i| path.join(format!("{}_{}", filename, i))).collect::<Vec<_>>();

    let files = paths
        .iter()
        .map(|p| {
            let file = HugepageMemFile(p);
            file.create_and_allocate(size.as_u64())?;
            Ok(file)
        })
        .collect::<Result<Vec<_>>>()?;
    // All files are created successfully, which means the initialization of
    // the hugepage memory files is successful, avoid destroying any file
    files.into_iter().for_each(mem::forget);
    Ok(paths)
}

struct HugepageMemFile<'a>(&'a Path);

impl<'a> HugepageMemFile<'a> {
    fn create_and_allocate(&self, size: u64) -> Result<()> {
        let file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(self.0)
            .context("create hugepage memory file")?;
        allocate_file(&file, size).context("allocate file")
    }
}

impl<'a> Drop for HugepageMemFile<'a> {
    fn drop(&mut self) {
        if let Err(e) = remove_file(self.0) {
            warn!(err=?e, "Unable to destroy file: '{}'", self.0.display());
        }
    }
}

fn allocate_file(file: &File, size: u64) -> io::Result<()> {
    use std::os::unix::io::AsRawFd;

    // file.set_len(size)?;

    let fd = file.as_raw_fd();
    let size: libc::off_t = size.try_into().unwrap();
    let _ = cvt(unsafe { libc::fallocate(fd, 0, 0, size) })?;
    Ok(())
}

trait IsMinusOne {
    fn is_minus_one(&self) -> bool;
}

macro_rules! impl_is_minus_one {
    ($($t:ident)*) => ($(impl IsMinusOne for $t {
        fn is_minus_one(&self) -> bool {
            *self == -1
        }
    })*)
}

impl_is_minus_one! { i8 i16 i32 i64 isize }

fn cvt<T: IsMinusOne>(t: T) -> std::io::Result<T> {
    if t.is_minus_one() {
        Err(std::io::Error::last_os_error())
    } else {
        Ok(t)
    }
}

fn cvt_r<T, F>(mut f: F) -> std::io::Result<T>
where
    T: IsMinusOne,
    F: FnMut() -> T,
{
    loop {
        match cvt(f()) {
            Err(ref e) if e.kind() == std::io::ErrorKind::Interrupted => {}
            other => return other,
        }
    }
}
