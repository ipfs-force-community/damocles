//! This demo shows how to implement a customize processor for a builtin task.
//!
//! $ cargo run --example customize-proc --features="ext-producer builtin-tasks"
//! ```
//! 2022-06-02T05:34:31.705303Z  INFO sub{name=tree_d pid=2824}: vc_processors::core::ext::consumer: processor ready
//! 2022-06-02T05:34:31.705457Z  INFO parent{pid=509}: vc_processors::core::ext::producer: producer ready
//! 2022-06-02T05:34:31.705714Z  INFO parent{pid=509}: customize_proc: producer start child=2824
//! 2022-06-02T05:34:31.705858Z  INFO parent{pid=509}: customize_proc: please enter a dir:
//! a
//! 2022-06-02T05:34:34.411353Z  INFO parent{pid=509}: customize_proc: token acquired
//! 2022-06-02T05:34:34.412432Z  INFO request{id=0 size=99}: customize_proc: process tree_d task dir="a"
//! 2022-06-02T05:34:37.413271Z  INFO parent{pid=509}: customize_proc: do nothing
//! 2022-06-02T05:34:37.413492Z  INFO parent{pid=509}: customize_proc: get output: false
//! 2022-06-02T05:34:37.413632Z  INFO parent{pid=509}: customize_proc: please enter a dir:
//! ```

use std::env::{self, current_exe};
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use filecoin_proofs_api::RegisteredSealProof;
use tokio::io::{self, AsyncBufReadExt, BufReader};
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_ipc::client::Client as IpcClient;
use vc_ipc::framed::{FramedExt, LinesCodec};
use vc_ipc::readwriter::{pipe, ReadWriterExt};
use vc_ipc::serded::Json;
use vc_ipc::server::Server as IpcServer;
use vc_processors::builtin::executor::TaskExecutor;
use vc_processors::builtin::simple_processor::running_store::MemoryRunningStore;
use vc_processors::builtin::simple_processor::SimpleProcessor;
use vc_processors::builtin::task_id_extractor::Md5BincodeTaskIdExtractor;
use vc_processors::builtin::tasks::TreeD;
use vc_processors::core::{ProcessorClient, Task, TowerServiceWrapper};

#[derive(Clone, Copy, Default)]
struct TreeDProc;

impl TaskExecutor<TreeD> for TreeDProc {
    fn exec(&self, task: TreeD) -> Result<<TreeD as Task>::Output> {
        info!(dir = ?task.cache_dir, "process tree_d task");
        std::thread::sleep(Duration::from_secs(3));
        Ok(false)
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(fmt::layer().with_writer(std::io::stderr))
        .with(
            EnvFilter::builder()
                .with_default_directive(LevelFilter::DEBUG.into())
                .from_env()
                .context("env filter")?,
        )
        .init();

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
        TreeDProc::default(),
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
        info!("please enter a dir:");

        let line = match reader.next_line().await.context("read line from stdin")? {
            Some(line) => line,
            None => {
                info!("exit");
                return Ok(());
            }
        };

        let loc = line.as_str().trim();
        if loc.is_empty() {
            info!("get empty location");
            return Ok(());
        }

        let dir = PathBuf::from(loc);
        let staged_file_path = dir.join("staged");

        let task_id = client
            .start_task(TreeD {
                registered_proof: RegisteredSealProof::StackedDrg2KiBV1_1,
                staged_file: staged_file_path,
                cache_dir: dir.clone(),
            })
            .await
            .context("start task")?;

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
