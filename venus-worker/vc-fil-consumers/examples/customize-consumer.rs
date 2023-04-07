// //! This demo shows how to implement a customize processor for a builtin task.
// //!
// //! $ cargo run --example customize-consumer
// //! ```
// //! 2022-06-02T05:34:31.705303Z  INFO sub{name=tree_d pid=2824}: vc_processors::core::ext::consumer: processor ready
// //! 2022-06-02T05:34:31.705457Z  INFO parent{pid=509}: vc_processors::core::ext::producer: producer ready
// //! 2022-06-02T05:34:31.705714Z  INFO parent{pid=509}: customize_proc: producer start child=2824
// //! 2022-06-02T05:34:31.705858Z  INFO parent{pid=509}: customize_proc: please enter a dir:
// //! a
// //! 2022-06-02T05:34:34.411353Z  INFO parent{pid=509}: customize_proc: token acquired
// //! 2022-06-02T05:34:34.412432Z  INFO request{id=0 size=99}: customize_proc: process tree_d task dir="a"
// //! 2022-06-02T05:34:37.413271Z  INFO parent{pid=509}: customize_proc: do nothing
// //! 2022-06-02T05:34:37.413492Z  INFO parent{pid=509}: customize_proc: get output: false
// //! 2022-06-02T05:34:37.413632Z  INFO parent{pid=509}: customize_proc: please enter a dir:
// //! ```

use std::env::{self, current_exe};
use std::future::Ready;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use filecoin_proofs_api::RegisteredSealProof;
use futures_util::{future::Map, FutureExt};
use tokio::io::{AsyncBufReadExt, BufReader};
use tower::ServiceBuilder;
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_fil_consumers::tasks::TreeD;
use vc_fil_consumers::{run_consumer, BuiltinExecutor};
use vc_processors::middleware::limit::delay::DelayLayer;
use vc_processors::producer::Producer;
use vc_processors::transport::default::{connect, pipe};
use vc_processors::util::ProcessorExt;
use vc_processors::{ready_msg, Consumer, ProcessorClient};

#[derive(Clone, Copy, Default)]
struct TreeDConsumer;

impl Consumer<TreeD> for TreeDConsumer {
    type TaskId = u64;
    type Error = anyhow::Error;
    type StartTaskFut = Ready<Result<Self::TaskId>>;
    type WaitTaskFut = Map<tokio::time::Sleep, fn(()) -> Result<Option<bool>, Self::Error>>;

    fn start_task(&mut self, task: TreeD) -> Self::StartTaskFut {
        info!(dir = ?task.cache_dir, "process tree_d task");
        std::future::ready(Ok(1))
    }

    fn wait_task(&mut self, _task_id: Self::TaskId) -> Self::WaitTaskFut {
        tokio::time::sleep(Duration::from_secs(3)).map(|_| Ok(Some(false)))
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
    if args.len() == 2 && args[1] == "consumer" {
        run_consumer::<TreeD, BuiltinExecutor>().await
    } else {
        run_producer().await
    }
}

async fn run_producer() -> Result<()> {
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
        .layer(DelayLayer::new(Duration::from_millis(200)))
        .service(Producer::<_, vc_fil_consumers::TaskId>::new(transport).spawn().tower());
    let mut client = ProcessorClient::new(producer);
    // info!(child = producer.child_pid(), "producer start");

    let mut reader = BufReader::new(tokio::io::stdin()).lines();

    loop {
        info!("please enter a dir:");

        let line = match reader.next_line().await.context("read line from stdin")? {
            Some(x) => x,
            None => {
                tracing::info!("exit");
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
        };
    }
}
