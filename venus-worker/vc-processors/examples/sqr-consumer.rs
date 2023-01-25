//! This demo shows how to implement an ext consumer for some specific `Task`.
//!
//! You can run the compiled binary, enter something like `{"id": 1, "task": 15}` to the input,
//! and then see the result from stdout.
//!
//! ```
//! $ cargo run --example sqr-consumer
//! 2022-06-01T07:41:34.114535Z  INFO sqr_consumer: start sqr consumer
//! pow processor ready
//! 2022-06-01T07:41:34.114678Z  INFO sub{name=pow pid=10951}: vc_processors::core::ext::consumer: processor ready
//! {"id": 1, "task": 15}
//! {"id":1,"err_msg":null,"output":225}
//! ```
//!
//! You can use any number you like to replace `15`, and see what happens.
//!

use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use serde::{Deserialize, Serialize};
use tracing::info;
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_ipc::{
    framed::{FramedExt, LinesCodec},
    readwriter::{pipe, ReadWriterExt},
    serded::Json,
    server::Server as IpcServer,
};
use vc_processors::{
    builtin::{
        executor::TaskExecutor,
        simple_processor::{running_store::MemoryRunningStore, SimpleProcessor},
        task_id_extractor::Md5BincodeTaskIdExtractor,
    },
    core::{Task, TowerServiceWrapper},
};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(transparent)]
struct Num(pub u32);

impl Task for Num {
    const STAGE: &'static str = "sqr";
    type Output = Num;
}

#[derive(Copy, Clone, Default)]
struct PowExecutor;

impl TaskExecutor<Num> for PowExecutor {
    fn exec(&self, task: Num) -> Result<<Num as Task>::Output> {
        if task.0 > 10086 {
            return Err(anyhow!("too large!!"));
        }
        Ok(Num(task.0.pow(2)))
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    init_log()?;
    info!("start sqr consumer");

    let transport = pipe::listen().framed(LinesCodec::default()).serded(Json::default());

    let sp = SimpleProcessor::new(
        Md5BincodeTaskIdExtractor::default(),
        PowExecutor::default(),
        MemoryRunningStore::new(Duration::from_secs(3600 * 24 * 2)).spawn(),
    )
    .spawn();

    let processor_service = tower::ServiceBuilder::new()
        .buffer(10)
        .rate_limit(10, Duration::from_secs(1))
        .concurrency_limit(5)
        .service(TowerServiceWrapper(sp));

    let ipc_server: IpcServer<_, _, _> = (transport, processor_service).into();
    ipc_server.serve().await.context("ipc server error")
}

fn init_log() -> Result<()> {
    tracing_subscriber::registry()
        .with(fmt::layer())
        .with(
            EnvFilter::builder()
                .with_default_directive(LevelFilter::DEBUG.into())
                .from_env()
                .context("env filter")?,
        )
        .init();
    Ok(())
}
