//! This demo shows how to use builtin tasks & processors.
//!
//! Also, it shows how to set a simple rate limiter with hooks.
//!
//! ```
//! $ cargo run --example builtin-proc
//! 2023-04-06T07:47:46.358751Z  INFO parent{pid=50078}: builtin_executor: please enter a dir:
//! 2023-04-06T07:47:46.376978Z  INFO parent{pid=50078}:child process{child_pid=50095}: tokio_transports::rw::pipe::connect: child process ready
//! a 
//! 2023-04-06T07:52:28.547375Z  INFO parent{pid=50078}: builtin_executor: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="a"
//! 2023-04-06T07:52:28.602282Z  INFO parent{pid=50078}: builtin_executor: get output: true
//! 2023-04-06T07:52:28.603114Z  INFO parent{pid=50078}: builtin_executor: please enter a dir:
//! b
//! 2023-04-06T07:52:49.475230Z  INFO parent{pid=50078}: builtin_executor: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="b"
//! 2023-04-06T07:52:49.512078Z  INFO parent{pid=50078}: builtin_executor: get output: true
//! 2023-04-06T07:52:49.512543Z  INFO parent{pid=50078}: builtin_executor: please enter a dir:
//! c
//! 2023-04-06T07:52:49.922744Z  INFO parent{pid=50078}: builtin_executor: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="c"
//! 2023-04-06T07:52:49.958957Z  INFO parent{pid=50078}: builtin_executor: get output: true
//! 2023-04-06T07:52:49.959354Z  INFO parent{pid=50078}: builtin_executor: please enter a dir:
//! d
//! 2023-04-06T07:52:50.339628Z  INFO parent{pid=50078}: builtin_executor: will generate an empty staged file of 8MiB & a tree_d file, and clean them up after dir="d"
//! 2023-04-06T07:52:50.375916Z  INFO parent{pid=50078}: builtin_executor: get output: true
//! 2023-04-06T07:52:50.376764Z  INFO parent{pid=50078}: builtin_executor: please enter a dir:
//! ```

use std::env::{self, current_exe};
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use filecoin_proofs_api::RegisteredSealProof;
use tokio::fs::{create_dir_all, remove_dir_all, OpenOptions};
use tokio::io::{AsyncBufReadExt, BufReader};
use tower::ServiceBuilder;
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_fil_consumers::builtin::executors::BuiltinExecutor;
use vc_fil_consumers::run_consumer;
use vc_fil_consumers::tasks::TreeD;
use vc_processors_v2::middleware::limit::delay::DelayLayer;
use vc_processors_v2::producer::Producer;
use vc_processors_v2::transport::default::{connect, pipe};
use vc_processors_v2::util::ProcessorExt;
use vc_processors_v2::{ready_msg, ProcessorClient};

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
    if args.len() == 2 && args[1] == "consumer" {
        run_consumer::<TreeD, BuiltinExecutor>().await
    } else {
        run_producer().await
    }
}

async fn run_producer() -> anyhow::Result<()> {
    let _span = warn_span!("parent", pid = std::process::id()).entered();
    let program = current_exe().context("get current exe")?;

    let transport = connect(
        pipe::Command::new(program)
            .args(vec!["consumer".to_owned()])
            .auto_restart(true)
            .ready_message(ready_msg::<TreeD>())
            .ready_timeout(Duration::from_secs(5)),
    );
    let producer = ServiceBuilder::new()
        .concurrency_limit(1)
        .layer(DelayLayer::new(Duration::from_millis(10)))
        .service(Producer::<_, vc_fil_consumers::TaskId>::new(transport).spawn().tower());
    let mut client = ProcessorClient::new(producer);

    let mut reader = BufReader::new(tokio::io::stdin()).lines();

    loop {
        tracing::info!("please enter a dir:");

        let line = match reader.next_line().await.context("read line from stdin")? {
            Some(x) => x,
            None => {
                tracing::info!("exit");
                return Ok(());
            }
        };

        let loc = line.as_str().trim();
        if loc.is_empty() {
            warn!("get empty location");
            return Ok(());
        }

        let dir = PathBuf::from(loc);
        info!(
            ?dir,
            "will generate an empty staged file of 8MiB & a tree_d file, and clean them up after"
        );

        create_dir_all(&dir).await.with_context(|| format!("create dir at {:?}", &dir))?;
        let staged_file_path = dir.join("staged");

        let fs = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(&staged_file_path)
            .await
            .context("create staged file")?;
        fs.set_len(2 << 10).await.context("set len for staged file")?;

        match client
            .process(TreeD {
                registered_proof: RegisteredSealProof::StackedDrg2KiBV1_1,
                staged_file: staged_file_path,
                cache_dir: dir.clone(),
            })
            .await
        {
            Ok(out) => {
                info!("get output: {:?}", out);
            }
            Err(e) => {
                warn!("get err: {:?}", e);
            }
        }

        remove_dir_all(dir).await.context("remove the demo dir")?;
    }
}
