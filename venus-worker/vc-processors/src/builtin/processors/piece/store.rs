use std::{
    convert::Infallible,
    fmt::Debug,
    io::{self, Read},
};

use anyhow::{anyhow, Context};

use crate::builtin::tasks::PieceFile;

mod http;
mod local;

/// Attempts to open a piece file
pub fn open(piece_file: PieceFile, payload_size: u64, target_size: u64) -> anyhow::Result<Box<dyn io::Read>> {
    let r = match piece_file {
        PieceFile::Url(u) => inflator(http::store_ref(), payload_size, target_size).open(u),
        PieceFile::Local(p) => inflator(local::store_ref(), payload_size, target_size).open(p),
        PieceFile::Pledge => inflator(pledge_store_ref(), payload_size, target_size).open(()),
    };

    r.context("build inflator reader")
}

pub trait PieceStore {
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

impl<T> PieceStore for Inflator<'_, T>
where
    T: PieceStore,
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
        Ok(if self.target_size != self.payload_size {
            Box::new(
                r.take(self.payload_size)
                    .chain(io::repeat(0).take(self.target_size - self.payload_size)),
            )
        } else {
            Box::new(r.take(self.payload_size))
        })
    }
}

pub fn pledge_store_ref() -> &'static PledgeStore {
    static X: PledgeStore = PledgeStore;
    &X
}

pub struct PledgeStore;

impl PieceStore for PledgeStore {
    type P = ();
    type Err = Infallible;
    type Read = io::Repeat;

    fn open(&self, _: Self::P) -> Result<Self::Read, Self::Err> {
        Ok(io::repeat(0))
    }
}
