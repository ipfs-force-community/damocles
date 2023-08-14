//! processor abstractions & implementations for sealing

use std::{fmt, marker::PhantomData, ops::Deref};

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

pub trait LockedProcessor<T, P, G> {
    fn wait(&self) -> Result<Guard<T, P, G>>;
}

#[derive(Debug, Clone)]
pub enum Either<L, R> {
    Left(L),
    Right(R),
}

impl<T, P, L, R, LG, RG> LockedProcessor<T, P, Either<LG, RG>> for Either<L, R>
where
    T: Task,
    P: Processor<T>,
    L: LockedProcessor<T, P, LG>,
    R: LockedProcessor<T, P, RG>,
{
    fn wait(&self) -> Result<Guard<'_, T, P, Either<LG, RG>>> {
        Ok(match self {
            Either::Left(p) => {
                let Guard { p, inner_guard, .. } = p.wait()?;
                Guard {
                    p,
                    inner_guard: Either::Left(inner_guard),
                    _ph: PhantomData,
                }
            }
            Either::Right(p) => {
                let Guard { p, inner_guard, .. } = p.wait()?;
                Guard {
                    p,
                    inner_guard: Either::Right(inner_guard),
                    _ph: PhantomData,
                }
            }
        })
    }
}

#[derive(Debug, Clone)]
pub struct NoLockProcessor<P>(P);

impl<P> NoLockProcessor<P> {
    pub fn new(inner: P) -> Self {
        Self(inner)
    }
}

impl<T: Task, P: Processor<T>> LockedProcessor<T, P, ()> for NoLockProcessor<P> {
    fn wait(&self) -> Result<Guard<'_, T, P, ()>> {
        Ok(Guard {
            p: &self.0,
            inner_guard: (),
            _ph: PhantomData,
        })
    }
}

pub struct Guard<'a, T, P, G> {
    p: &'a P,
    inner_guard: G,
    _ph: PhantomData<T>,
}

impl<T, P, G> Deref for Guard<'_, T, P, G> {
    type Target = P;

    fn deref(&self) -> &Self::Target {
        self.p
    }
}

impl<T: Task, P: Processor<T>, G> fmt::Display for Guard<'_, T, P, G> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.p.name().as_str())
    }
}
