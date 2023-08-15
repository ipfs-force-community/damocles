//! processor abstractions & implementations for sealing

use anyhow::Result;
pub use vc_processors::{
    builtin::tasks::{
        AddPieces as AddPiecesInput, SnapEncode as SnapEncodeInput, SnapProve as SnapProveInput, Transfer as TransferInput, TransferItem,
        TransferOption, TransferRoute, TransferStoreInfo, TreeD as TreeDInput, Unseal as UnsealInput, WindowPoSt as WindowPoStInput,
        C2 as C2Input, PC1 as PC1Input, PC2 as PC2Input, STAGE_NAME_C1, STAGE_NAME_C2, STAGE_NAME_PC1, STAGE_NAME_PC2,
        STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
    },
    core::{Processor, Task},
};

pub mod external;
mod safe;
pub use safe::*;

pub trait LockProcessor {
    type Guard<'a>
    where
        Self: 'a;

    fn lock(&self) -> Result<Self::Guard<'_>>;
}

#[derive(Debug, Clone)]
pub enum Either<L, R> {
    Left(L),
    Right(R),
}

impl<L, R> LockProcessor for Either<L, R>
where
    L: LockProcessor,
    R: LockProcessor,
{
    type Guard<'a> = Either<L::Guard<'a>, R::Guard<'a>>
    where
        Self: 'a;

    fn lock(&self) -> Result<Self::Guard<'_>> {
        Ok(match self {
            Either::Left(p) => Either::Left(p.lock()?),
            Either::Right(p) => Either::Right(p.lock()?),
        })
    }
}

impl<T, L, R> std::ops::Deref for Either<L, R>
where
    L: std::ops::Deref<Target = T>,
    R: std::ops::Deref<Target = T>,
{
    type Target = T;

    fn deref(&self) -> &Self::Target {
        match self {
            Either::Left(x) => x.deref(),
            Either::Right(x) => x.deref(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct NoLockProcessor<P>(P);

impl<P> NoLockProcessor<P> {
    pub fn new(inner: P) -> Self {
        Self(inner)
    }
}

impl<P> LockProcessor for NoLockProcessor<P> {
    type Guard<'a> = &'a P where P: 'a;

    fn lock(&self) -> Result<&P> {
        Ok(&self.0)
    }
}
