#[cfg(feature = "b64serde")]
pub mod b64serde;

pub mod builtin;
#[allow(missing_docs)]
#[cfg(feature = "fil-proofs")]
pub mod fil_proofs;

#[allow(missing_docs)]
pub mod tasks;

use anyhow::Context;

pub use builtin::executors::BuiltinExecutor;
use vc_processors_v2::{
    consumer::{serve, DefaultConsumer, Executor},
    ready_msg,
    transport::default::listen_ready_message,
    ConsumerError, Task,
};

/// Default type of task id
pub type TaskId = u64;

// Run consumer
pub async fn run_consumer<Tsk, Exe>() -> anyhow::Result<()>
where
    Tsk: Task + Unpin,
    <Tsk as Task>::Output: Unpin,
    Exe: Executor<Tsk> + Default + Send + 'static,
    Exe::Error: Into<ConsumerError> + Send,
{
    serve(
        listen_ready_message(ready_msg::<Tsk>()).await.context("write ready message")?,
        DefaultConsumer::<Tsk, _, _>::new_xxhash(Exe::default()),
    )
    .await?;
    Ok(())
}
