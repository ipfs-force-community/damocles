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
use vc_processors::{
    consumer::{serve, DefaultConsumer, Executor},
    ready_msg,
    transport::default::listen_ready_message,
    ConsumerError, Task,
};

pub type TaskId = u64;

pub async fn run_consumer<Tsk, Exe>() -> anyhow::Result<()>
where
    Tsk: Task,
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
