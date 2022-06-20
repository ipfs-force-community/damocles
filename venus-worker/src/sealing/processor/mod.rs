//! processor abstractions & implementations for sealing

pub use vc_processors::{
    builtin::tasks::{
        SnapEncode as SnapEncodeInput, SnapProve as SnapProveInput, TranferItem, Transfer as TransferInput, TransferOption, TransferRoute,
        TransferStoreInfo, TreeD as TreeDInput, C2 as C2Input, PC1 as PC1Input, PC2 as PC2Input, STAGE_NAME_C1, STAGE_NAME_C2,
        STAGE_NAME_PC1, STAGE_NAME_PC2, STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
    },
    core::{Processor, Task as Input},
};

pub mod external;
mod safe;
pub use safe::*;

pub type BoxedProcessor<I> = Box<dyn Processor<I>>;
pub type BoxedTreeDProcessor = BoxedProcessor<TreeDInput>;
pub type BoxedPC1Processor = BoxedProcessor<PC1Input>;
pub type BoxedPC2Processor = BoxedProcessor<PC2Input>;
pub type BoxedC2Processor = BoxedProcessor<C2Input>;
pub type BoxedSnapEncodeProcessor = BoxedProcessor<SnapEncodeInput>;
pub type BoxedSnapProveProcessor = BoxedProcessor<SnapProveInput>;
pub type BoxedTransferProcessor = BoxedProcessor<TransferInput>;
