use std::{fs, io, path::Path};

use super::PieceFetcher;

/// Returns the static reference to the `LocalFileFetcher`
pub fn fetcher_ref() -> &'static LocalFileFetcher {
    static X: LocalFileFetcher = LocalFileFetcher;
    &X
}

/// A PieceFetcher for the local file
pub struct LocalFileFetcher;

impl<P: AsRef<Path>> PieceFetcher<P> for LocalFileFetcher {
    type Err = io::Error;
    type Read = fs::File;

    fn open(&self, p: P) -> Result<Self::Read, Self::Err> {
        fs::File::open(p)
    }
}
