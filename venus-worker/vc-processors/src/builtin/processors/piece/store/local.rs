use std::{fs, io, path::Path};

use super::PieceStore;

/// Returns the static reference to the `LocalFileStore`
pub fn store_ref() -> &'static LocalFileStore {
    static X: LocalFileStore = LocalFileStore;
    &X
}

/// A PieceStore for the local file
pub struct LocalFileStore;

impl<P: AsRef<Path>> PieceStore<P> for LocalFileStore {
    type Err = io::Error;
    type Read = fs::File;

    fn open(&self, p: P) -> Result<Self::Read, Self::Err> {
        fs::File::open(p)
    }
}
