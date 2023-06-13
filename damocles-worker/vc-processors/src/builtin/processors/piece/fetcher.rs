use std::{
    convert::Infallible,
    fmt::Debug,
    io::{self, Read},
};

use anyhow::{anyhow, Context};

use crate::builtin::tasks::PieceFile;

pub mod http;
pub mod local;

/// Attempts to open a piece file
pub fn open(piece_file: PieceFile, payload_size: u64, target_size: u64) -> anyhow::Result<Box<dyn io::Read>> {
    let r = match piece_file {
        PieceFile::Url(u) => inflator(http::fetcher_ref(), payload_size, target_size).open(u),
        PieceFile::Local(p) => inflator(local::fetcher_ref(), payload_size, target_size).open(p),
        PieceFile::Pledge => inflator(pledge_fetcher_ref(), payload_size, target_size).open(()),
    };

    r.context("build inflator reader")
}

/// A piece fetcher. intended for open piece file from different fetcher implementations.
pub trait PieceFetcher<Addr> {
    type Err: Debug;
    type Read: io::Read;

    fn open(&self, addr: Addr) -> Result<Self::Read, Self::Err>;
}

/// Creates a new `Inflator<T>`
pub fn inflator<T>(inner: &T, payload_size: u64, target_size: u64) -> Inflator<T> {
    Inflator {
        inner,
        payload_size,
        target_size,
    }
}

/// Inflator opens the piece file by the given `PieceFetcher`
/// and fills the end of the returned `io::Read` with NUL bytes
/// until the returned `io::Read` length reaches the given target_size.
pub struct Inflator<'a, T> {
    inner: &'a T,
    payload_size: u64,
    target_size: u64,
}

impl<Addr, T> PieceFetcher<Addr> for Inflator<'_, T>
where
    T: PieceFetcher<Addr>,
    T::Read: 'static,
{
    type Err = anyhow::Error;
    type Read = Box<dyn io::Read>;

    fn open(&self, addr: Addr) -> Result<Self::Read, Self::Err> {
        if self.payload_size > self.target_size {
            return Err(anyhow!("payload size larger than target size"));
        }

        let r = self.inner.open(addr).map_err(|e| anyhow!("open inner. {:?}", e))?;
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

/// Returns the static reference to the `PledgeSFetcher`
pub fn pledge_fetcher_ref() -> &'static PledgeFetcher {
    static X: PledgeFetcher = PledgeFetcher;
    &X
}

/// A pledge piece fetcher.
/// The open method of the `PledgeFetcher` always returns the `io::Read` with full NUL bytes
/// and does not generate any errors.
pub struct PledgeFetcher;

impl PieceFetcher<()> for PledgeFetcher {
    type Err = Infallible;
    type Read = io::Repeat;

    fn open(&self, _: ()) -> Result<Self::Read, Self::Err> {
        Ok(io::repeat(0))
    }
}
