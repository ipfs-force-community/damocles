//! Built-in executors

#[allow(missing_docs)]
pub mod executors;

pub mod local_thread_processor {

    macro_rules! gen_local_thread_processor {
        ($name:ident, $task:ty) => {
            pub fn $name() -> vc_processors_v2::ThreadProcessor<$task, crate::builtin::executors::BuiltinExecutor> {
                vc_processors_v2::ThreadProcessor::new(crate::builtin::executors::BuiltinExecutor {})
            }
        };
    }

    gen_local_thread_processor!(add_pieces, crate::tasks::AddPieces);
    gen_local_thread_processor!(pc1, crate::tasks::PC1);
    gen_local_thread_processor!(pc2, crate::tasks::PC2);
    gen_local_thread_processor!(c2, crate::tasks::C2);
    gen_local_thread_processor!(snap_encode, crate::tasks::SnapEncode);
    gen_local_thread_processor!(transfer, crate::tasks::Transfer);
    gen_local_thread_processor!(window_post, crate::tasks::WindowPoSt);
    gen_local_thread_processor!(winning_post, crate::tasks::WinningPoSt);
}
