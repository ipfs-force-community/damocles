use std::{fs, io, path::PathBuf};

use super::PieceStore;

pub fn store_ref() -> &'static LocalFile {
    static X: LocalFile = LocalFile;
    &X
}

pub struct LocalFile;

impl PieceStore for LocalFile {
    type P = PathBuf;
    type Err = io::Error;
    type Read = fs::File;

    fn open(&self, p: Self::P) -> Result<Self::Read, Self::Err> {
        fs::File::open(&p)
    }
}
