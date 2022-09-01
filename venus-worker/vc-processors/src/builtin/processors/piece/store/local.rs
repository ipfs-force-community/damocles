use std::{fs, io, path::Path};

use super::PieceStore;

pub fn store_ref() -> &'static LocalFile {
    static X: LocalFile = LocalFile;
    &X
}

pub struct LocalFile;

impl<P: AsRef<Path>> PieceStore<P> for LocalFile {
    type Err = io::Error;
    type Read = fs::File;

    fn open(&self, p: P) -> Result<Self::Read, Self::Err> {
        fs::File::open(p)
    }
}
