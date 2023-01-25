//! This demo shows how to implement a pair of producer and consumer for some specific `Task`,
//! and make the producer act as a `Processor`.
//!
//! You can run the compiled binary, enter numbers one line each, and see the results.
//!
//! ```
//! $ cargo run --example sqr-producer --features=ext-producer
//! 2022-06-01T09:57:46.873986Z  INFO sub{name=pow pid=21576}: vc_processors::core::ext::consumer: processor ready
//! 2022-06-01T09:57:46.874102Z  INFO parent{pid=21575}: vc_processors::core::ext::producer: producer ready
//! 2022-06-01T09:57:46.874238Z  INFO parent{pid=21575}: sqr_producer: producer start child=21576
//! 2022-06-01T09:57:46.874327Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
//! 5
//! 2022-06-01T09:57:48.304364Z  INFO parent{pid=21575}: sqr_producer: read in num=5
//! 2022-06-01T09:57:48.304923Z  INFO parent{pid=21575}: sqr_producer: get output: Num(25)
//! 2022-06-01T09:57:48.304967Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
//! 7
//! 2022-06-01T09:57:49.654865Z  INFO parent{pid=21575}: sqr_producer: read in num=7
//! 2022-06-01T09:57:49.655199Z  INFO parent{pid=21575}: sqr_producer: get output: Num(49)
//! 2022-06-01T09:57:49.655230Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
//! 10
//! 2022-06-01T09:57:51.044910Z  INFO parent{pid=21575}: sqr_producer: read in num=10
//! 2022-06-01T09:57:51.045184Z  INFO parent{pid=21575}: sqr_producer: get output: Num(100)
//! 2022-06-01T09:57:51.045212Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
//! 100000
//! 2022-06-01T09:57:53.320905Z  INFO parent{pid=21575}: sqr_producer: read in num=100000
//! 2022-06-01T09:57:53.321201Z  WARN parent{pid=21575}: sqr_producer: get err: too large!!
//! 2022-06-01T09:57:53.321234Z  INFO parent{pid=21575}: sqr_producer: please enter a number:
//! ```
//!

use std::env::{self, current_exe};
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use serde::{Deserialize, Serialize};
use tokio::{
    io,
    io::{AsyncBufReadExt, BufReader},
};
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_ipc::{
    client::Client as IpcClient,
    framed::{FramedExt, LinesCodec},
    readwriter::{pipe, ReadWriterExt},
    serded::Json,
    server::Server as IpcServer,
};
use vc_processors::builtin::executor::TaskExecutor;
use vc_processors::builtin::simple_processor::running_store::MemoryRunningStore;
use vc_processors::builtin::simple_processor::SimpleProcessor;
use vc_processors::builtin::task_id_extractor::Md5BincodeTaskIdExtractor;
use vc_processors::core::{ProcessorClient, Task, TowerServiceWrapper};

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

    let args = env::args().collect::<Vec<String>>();
    if args.len() == 2 && args[1] == "sub" {
        return run_sub().await;
    };

    run_main().await
}

async fn run_sub() -> Result<()> {
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

async fn run_main() -> Result<()> {
    let _span = warn_span!("parent", pid = std::process::id()).entered();

    let transport = pipe::connect(current_exe().context("get current exe")?, vec!["sub".to_owned()])
        .context("connect to pipe")?
        .framed(LinesCodec::default())
        .serded(Json::default());

    let ipc_client = IpcClient::new(Default::default(), transport).spawn();
    let client = ProcessorClient::new(ipc_client);
    // info!(child = producer.child_pid(), "producer start");

    let mut reader = BufReader::new(io::stdin()).lines();

    loop {
        info!("please enter a number:");

        let line = match reader.next_line().await.context("read line from stdin")? {
            Some(line) => line,
            None => {
                info!("exit");
                return Ok(());
            }
        };

        let num: u32 = line
            .as_str()
            .trim()
            .parse()
            .with_context(|| format!("number required, got {}", line))?;
        info!(num, "read in");

        let task_id = client.start_task(Num(num)).await.context("start task")?;
        match client.wait_task(&task_id).await {
            Ok(out) => {
                info!("get output: {:?}", out);
            }

            Err(e) => {
                warn!("get err: {:?}", e);
            }
        };
    }
}

fn init_log() -> Result<()> {
    tracing_subscriber::registry()
        .with(fmt::layer().with_writer(std::io::stderr))
        .with(
            EnvFilter::builder()
                .with_default_directive(LevelFilter::DEBUG.into())
                .from_env()
                .context("env filter")?,
        )
        .init();
    Ok(())
}
