//! processor abstractions & implementations for sealing

use std::time::Duration;

pub use vc_fil_consumers::{
    builtin::local_thread_processor,
    tasks::{
        AddPieces, SnapEncode, SnapProve, Transfer, TransferItem, TransferOption, TransferRoute, TransferStoreInfo, TreeD, C2, PC1, PC2,
        STAGE_NAME_ADD_PIECES, STAGE_NAME_C1, STAGE_NAME_C2, STAGE_NAME_PC1, STAGE_NAME_PC2, STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE,
        STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
    },
};
use vc_processors::Task;

pub mod external;
mod safe;
pub use safe::*;

use self::external::ExtProcessor;

pub trait Client<T: Task> {
    fn process(&mut self, task: T) -> anyhow::Result<T::Output>;
}

#[derive(Debug, Clone)]
pub enum Either<A, B> {
    A(A),
    B(B),
}

impl<T: Task, A: Client<T>, B: Client<T>> Client<T> for Either<A, B> {
    fn process(&mut self, task: T) -> anyhow::Result<T::Output> {
        match self {
            Either::A(a) => a.process(task),
            Either::B(b) => b.process(task),
        }
    }
}

macro_rules! gen_client {
    ($task_name:ident) => {
        paste::paste! {
            #[derive(Debug, Clone)]
            pub struct [<$task_name LocalThreadClient>](
                vc_processors::ProcessorClient<
                $task_name,
                    vc_processors::util::tower::TowerWrapper<
                        vc_processors::ThreadProcessor<$task_name, vc_fil_consumers::builtin::executors::BuiltinExecutor>,
                    >,
                >,
            );

            impl Default for [<$task_name LocalThreadClient>] {
                fn default() -> Self {
                    use vc_processors::util::ProcessorExt;

                    Self(vc_processors::ProcessorClient::new(
                        vc_processors::ThreadProcessor::new(vc_fil_consumers::builtin::executors::BuiltinExecutor {}).tower(),
                    ))
                }
            }

            impl Client<$task_name> for [<$task_name LocalThreadClient>] {
                fn process(&mut self, task: $task_name) -> anyhow::Result<<$task_name as Task>::Output> {
                    crate::block_on(self.0.process(task)).map_err(|e| anyhow::anyhow!(e))
                }
            }

            pub type [<$task_name Processor>] = Either<ExtProcessor<$task_name>, [<$task_name LocalThreadClient>]>;


            pub fn [<create_ $task_name:snake _processor>](
                ext: &[self::external::config::Ext],
                delay: Option<Duration>,
                concurrent: Option<usize>,
                ext_locks: &std::collections::HashMap<String, std::sync::Arc<tokio::sync::Semaphore>>,
            ) -> anyhow::Result<[<$task_name Processor>]> {
                Ok(if !ext.is_empty() {
                    Either::A(self::external::ExtProcessor::build(ext, delay, concurrent, ext_locks)?)
                } else {
                    Either::B(Default::default())
                })
            }
        }
    };
}

gen_client!(AddPieces);
gen_client!(TreeD);
gen_client!(PC1);
gen_client!(PC2);
gen_client!(C2);
gen_client!(SnapEncode);
gen_client!(SnapProve);
gen_client!(Transfer);
