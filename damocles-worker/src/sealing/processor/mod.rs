//! processor abstractions & implementations for sealing

use std::sync::Arc;

pub use vc_processors::{
    builtin::tasks::{
        AddPieces as AddPiecesInput, SnapEncode as SnapEncodeInput, SnapProve as SnapProveInput, Transfer as TransferInput, TransferItem,
        TransferOption, TransferRoute, TransferStoreInfo, TreeD as TreeDInput, Unseal as UnsealInput, WindowPoSt, C2 as C2Input,
        PC1 as PC1Input, PC2 as PC2Input, STAGE_NAME_C1, STAGE_NAME_C2, STAGE_NAME_PC1, STAGE_NAME_PC2, STAGE_NAME_SNAP_ENCODE,
        STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
    },
    core::{Processor, Task as Input},
};

pub mod external;
mod safe;
pub use safe::*;

pub type ArcProcessor<I> = Arc<dyn Processor<I>>;
pub type ArcAddPiecesProcessor = ArcProcessor<AddPiecesInput>;
pub type ArcTreeDProcessor = ArcProcessor<TreeDInput>;
pub type ArcPC1Processor = ArcProcessor<PC1Input>;
pub type ArcPC2Processor = ArcProcessor<PC2Input>;
pub type ArcC2Processor = ArcProcessor<C2Input>;
pub type ArcSnapEncodeProcessor = ArcProcessor<SnapEncodeInput>;
pub type ArcSnapProveProcessor = ArcProcessor<SnapProveInput>;
pub type ArcTransferProcessor = ArcProcessor<TransferInput>;
pub type ArcUnsealProcessor = ArcProcessor<UnsealInput>;
pub type ArcWdPostProcessor = ArcProcessor<WindowPoSt>;
