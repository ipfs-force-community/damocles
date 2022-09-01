use std::{fs, io, path::PathBuf};

use super::PieceReader;

pub fn reader_ref() -> &'static LocalFile {
    static X: LocalFile = LocalFile;
    &X
}

pub struct LocalFile;

impl PieceReader for LocalFile {
    type P = PathBuf;
    type Err = io::Error;
    type Read = fs::File;

    fn open(&self, p: Self::P) -> Result<Self::Read, Self::Err> {
        fs::File::open(&p)
    }
}
