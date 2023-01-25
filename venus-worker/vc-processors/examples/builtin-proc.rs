//! This demo shows how to use builtin tasks & processors.
//!
//! Also, it shows how to set a simple rate limiter with hooks.
//!
//! ```
//! $ cargo run --example builtin-proc --features="ext-producer builtin"
//! 2022-06-01T09:53:30.693717Z  INFO sub{name=tree_d pid=21252}: vc_processors::core::ext::consumer: processor ready
//! 2022-06-01T09:53:30.693839Z  INFO parent{pid=21250}: vc_processors::core::ext::producer: producer ready
//! 2022-06-01T09:53:30.693959Z  INFO parent{pid=21250}: builtin_proc: producer start child=21252
//! 2022-06-01T09:53:30.694103Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! a
//! 2022-06-01T09:53:31.248063Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="a"
//! 2022-06-01T09:53:31.249032Z  INFO parent{pid=21250}: builtin_proc: token acquired
//! 2022-06-01T09:53:31.255737Z  INFO parent{pid=21250}: builtin_proc: do nothing
//! 2022-06-01T09:53:31.255784Z  INFO parent{pid=21250}: builtin_proc: get output: true
//! 2022-06-01T09:53:31.256044Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! b
//! 2022-06-01T09:53:31.994187Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="b"
//! 2022-06-01T09:53:35.686187Z  INFO builtin_proc: re-fill one token
//! 2022-06-01T09:53:35.686225Z  INFO parent{pid=21250}: builtin_proc: token acquired
//! 2022-06-01T09:53:35.688114Z  INFO parent{pid=21250}: builtin_proc: do nothing
//! 2022-06-01T09:53:35.688158Z  INFO parent{pid=21250}: builtin_proc: get output: true
//! 2022-06-01T09:53:35.688397Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! c
//! 2022-06-01T09:53:36.468706Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="c"
//! 2022-06-01T09:53:40.686604Z  INFO builtin_proc: re-fill one token
//! 2022-06-01T09:53:40.686655Z  INFO parent{pid=21250}: builtin_proc: token acquired
//! 2022-06-01T09:53:40.687885Z  INFO parent{pid=21250}: builtin_proc: do nothing
//! 2022-06-01T09:53:40.687928Z  INFO parent{pid=21250}: builtin_proc: get output: true
//! 2022-06-01T09:53:40.688241Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! d
//! 2022-06-01T09:53:45.481275Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="d"
//! 2022-06-01T09:53:45.686773Z  INFO builtin_proc: re-fill one token
//! 2022-06-01T09:53:45.686818Z  INFO parent{pid=21250}: builtin_proc: token acquired
//! 2022-06-01T09:53:45.688524Z  INFO parent{pid=21250}: builtin_proc: do nothing
//! 2022-06-01T09:53:45.688568Z  INFO parent{pid=21250}: builtin_proc: get output: true
//! 2022-06-01T09:53:45.688884Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! e2022-06-01T09:53:50.687008Z  INFO builtin_proc: re-fill one token
//!
//! 2022-06-01T09:53:53.877925Z  INFO parent{pid=21250}: builtin_proc: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="e"
//! 2022-06-01T09:53:53.878905Z  INFO parent{pid=21250}: builtin_proc: token acquired
//! 2022-06-01T09:53:53.880119Z  INFO parent{pid=21250}: builtin_proc: do nothing
//! 2022-06-01T09:53:53.880161Z  INFO parent{pid=21250}: builtin_proc: get output: true
//! 2022-06-01T09:53:53.880457Z  INFO parent{pid=21250}: builtin_proc: please enter a dir:
//! 2022-06-01T09:53:55.687241Z  INFO builtin_proc: re-fill one token
//! ```

use std::env::{self, current_exe};
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use tokio::fs::{create_dir_all, remove_dir_all, OpenOptions};
use tokio::io::{self, AsyncBufReadExt, BufReader};
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_ipc::client::Client as IpcClient;
use vc_ipc::framed::{FramedExt, LinesCodec};
use vc_ipc::readwriter::{pipe, ReadWriterExt};
use vc_ipc::serded::Json;
use vc_ipc::server::Server as IpcServer;
use vc_processors::builtin::simple_processor::running_store::MemoryRunningStore;
use vc_processors::builtin::simple_processor::SimpleProcessor;
use vc_processors::builtin::task_id_extractor::Md5BincodeTaskIdExtractor;
use vc_processors::{
    builtin::{executor::BuiltinTaskExecutor, tasks::TreeD},
    core::{ProcessorClient, TowerServiceWrapper},
    fil_proofs::RegisteredSealProof,
};

#[tokio::main]
async fn main() -> Result<()> {
    init_log()?;

    let args = env::args().collect::<Vec<String>>();
    if args.len() == 2 && args[1] == "sub" {
        return run_sub().await;
    };

    run_main().await
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

async fn run_sub() -> Result<()> {
    let transport = pipe::listen().framed(LinesCodec::default()).serded(Json::default());

    let sp = SimpleProcessor::<TreeD, _, _, _>::new(
        Md5BincodeTaskIdExtractor::default(),
        BuiltinTaskExecutor::default(),
        MemoryRunningStore::new(Duration::from_secs(3600 * 24 * 2)).spawn(),
    )
    .spawn();

    let processor_service = tower::ServiceBuilder::new()
        .buffer(10)
        .rate_limit(1, Duration::from_secs(1))
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
        info!(
            ?dir,
            "will generate an empty staged file of 8MiB & a tree_d file, and clean them up after"
        );

        let staged_file_path = dir.join("staged");
        create_dir_all(&dir).await.with_context(|| format!("create dir at {:?}", &dir))?;
        let fs = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(&staged_file_path)
            .await
            .context("create staged file")?;

        fs.set_len(2 << 10).await.context("set len for staged file")?;

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

        remove_dir_all(dir).await.context("remove the demo dir")?;
    }
}
