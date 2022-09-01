use std::{
    convert::Infallible,
    fmt::Debug,
    io::{self, Read},
};

use anyhow::{anyhow, Context};

use crate::builtin::tasks::PieceFile;

mod http;
mod local;

pub fn open(piece_file: PieceFile, payload_size: u64, target_size: u64) -> anyhow::Result<Box<dyn io::Read>> {
    let r = match piece_file {
        PieceFile::Url(u) => inflator(http::reader_ref(), payload_size, target_size).open(u),
        PieceFile::Local(p) => inflator(local::reader_ref(), payload_size, target_size).open(p),
        PieceFile::Pledge => inflator(pledge_reader_ref(), payload_size, target_size).open(()),
    };

    r.context("build inflator reader")
}

pub trait PieceReader {
    type P;
    type Err: Debug;
    type Read: io::Read;

    fn open(&self, p: Self::P) -> Result<Self::Read, Self::Err>;
}

fn inflator<T>(inner: &T, payload_size: u64, target_size: u64) -> Inflator<T> {
    Inflator {
        inner,
        payload_size,
        target_size,
    }
}

struct Inflator<'a, T> {
    inner: &'a T,
    payload_size: u64,
    target_size: u64,
}

impl<T> PieceReader for Inflator<'_, T>
where
    T: PieceReader,
    T::Read: 'static,
{
    type P = T::P;
    type Err = anyhow::Error;
    type Read = Box<dyn io::Read>;

    fn open(&self, p: Self::P) -> Result<Self::Read, Self::Err> {
        if self.payload_size > self.target_size {
            return Err(anyhow!("payload size larger than target size"));
        }

        let r = self.inner.open(p).map_err(|e| anyhow!("open inner. {:?}", e))?;
        Ok(Box::new(ioread_inflator(r, self.payload_size, self.target_size)) as Box<dyn io::Read>)
    }
}

fn ioread_inflator<R: io::Read>(inner: R, payload_size: u64, target_size: u64) -> impl io::Read {
    inner.take(payload_size).chain(io::repeat(0).take(target_size - payload_size))
}

pub fn pledge_reader_ref() -> &'static PledgeReader {
    static X: PledgeReader = PledgeReader;
    &X
}

pub struct PledgeReader;

impl PieceReader for PledgeReader {
    type P = ();
    type Err = Infallible;
    type Read = io::Repeat;

    fn open(&self, _: Self::P) -> Result<Self::Read, Self::Err> {
        Ok(io::repeat(0))
    }
}
