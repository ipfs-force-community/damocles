//! processor abstractions & implementations for sealing

use std::ops::Deref;

use anyhow::Result;
pub use vc_processors::{
    builtin::tasks::{
        AddPieces as AddPiecesInput, SnapEncode as SnapEncodeInput, SnapProve as SnapProveInput, Transfer as TransferInput, TransferItem,
        TransferOption, TransferRoute, TransferStoreInfo, TreeD as TreeDInput, Unseal as UnsealInput, WindowPoSt as WindowPoStInput,
        C2 as C2Input, PC1 as PC1Input, PC2 as PC2Input, STAGE_NAME_C1, STAGE_NAME_C2, STAGE_NAME_PC1, STAGE_NAME_PC2,
        STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
    },
    core::{Processor, Task as Input},
};

pub mod external;
mod safe;
pub use safe::*;

pub trait LockedProcesssor<P, G> {
    fn wait(&self) -> Result<Guard<P, G>>;
}

#[derive(Debug, Clone)]
pub enum Either<L, R> {
    Left(L),
    Right(R),
}

impl<I, L, LG, R, RG> LockedProcesssor<Box<dyn Processor<I>>, Either<LG, RG>> for Either<L, R>
where
    I: Input,
    L: LockedProcesssor<Box<dyn Processor<I>>, LG>,
    R: LockedProcesssor<Box<dyn Processor<I>>, RG>,
{
    fn wait(&self) -> Result<Guard<Box<dyn Processor<I>>, Either<LG, RG>>> {
        Ok(match self {
            Either::Left(p) => {
                let Guard { p, inner_guard } = p.wait()?;
                Guard {
                    p,
                    inner_guard: Either::Left(inner_guard),
                }
            }
            Either::Right(p) => {
                let Guard { p, inner_guard } = p.wait()?;
                Guard {
                    p,
                    inner_guard: Either::Right(inner_guard),
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

impl<P> LockedProcesssor<P, ()> for NoLockProcessor<P> {
    fn wait(&self) -> Result<Guard<P, ()>> {
        Ok(Guard {
            p: &self.0,
            inner_guard: (),
        })
    }
}

pub struct Guard<'a, P, G> {
    p: &'a P,
    inner_guard: G,
}

impl<P, G> Deref for Guard<'_, P, G> {
    type Target = P;

    fn deref(&self) -> &Self::Target {
        self.p
    }
}
