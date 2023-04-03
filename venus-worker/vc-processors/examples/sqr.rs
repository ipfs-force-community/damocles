//! This demo shows how to implement a pair of producer and consumer for some specific `Task`,
//! and make the producer act as a `Processor`.
//!
//! You can run the compiled binary, enter numbers one line each, and see the results.
//!
//! ```
//! $ RUST_LOG=info cargo run --example sqr
//!
//! 2023-04-03T06:39:18.244588Z  INFO parent{pid=18776}: sqr: please enter a number:
//! 2023-04-03T06:39:18.262109Z  WARN vc_processors::sys::cgroup::imp: macos does not support cgroups.
//! 2023-04-03T06:39:18.262344Z  INFO parent{pid=18776}:child process{child_pid=18777}: tokio_transports::rw::pipe::connect: child process ready
//! 5
//! pow(5) = Ok(Num(25))
//! 2023-04-03T06:39:20.255683Z  INFO parent{pid=18776}: sqr: please enter a number:
//! 7
//! pow(7) = Ok(Num(49))
//! 2023-04-03T06:39:23.801608Z  INFO parent{pid=18776}: sqr: please enter a number:
//! 10
//! pow(10) = Ok(Num(100))
//! 2023-04-03T06:39:29.699292Z  INFO parent{pid=18776}: sqr: please enter a number:
//! 100000
//! pow(100000) = Err(Consumer(ConsumerError { kind: InvalidInput, detail: "too large!!" }))
//! 2023-04-03T06:39:35.235818Z  INFO parent{pid=18776}: sqr: please enter a number:
//! ```
//!

use std::env::{self, current_exe};
use std::io;
use std::time::Duration;

use anyhow::Context as AnyhowContext;
use serde::{Deserialize, Serialize};
use tokio::io::{AsyncBufReadExt, BufReader};
use tower::ServiceBuilder;
use tracing::warn_span;
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::consumer::{DefaultConsumer, Executor};
use vc_processors::middleware::limit::delay::DelayLayer;
use vc_processors::producer::Producer;
use vc_processors::transport::default::{connect, listen_ready_message, pipe};
use vc_processors::util::ProcessorExt;
use vc_processors::{ready_msg, ProcessorClient, Task};

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(transparent)]
struct Num(pub u32);

impl Task for Num {
    const STAGE: &'static str = "pow";
    type Output = Num;
}

#[derive(Copy, Clone, Default)]
struct PowExecutor;

impl Executor<Num> for PowExecutor {
    type Error = io::Error;

    fn execute(&self, task: Num) -> Result<<Num as vc_processors::Task>::Output, Self::Error> {
        if task.0 > 10086 {
            return Err(io::Error::new(io::ErrorKind::InvalidInput, "too large!!"));
        }

        Ok(Num(task.0.pow(2)))
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
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
        run_consumer().await
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
            .ready_message(ready_msg::<Num>())
            .ready_timeout(Duration::from_secs(5)),
    );
    let producer = ServiceBuilder::new()
        .concurrency_limit(1)
        .layer(DelayLayer::new(Duration::from_millis(10)))
        .service(Producer::<_, u64>::new(transport).spawn().tower());
    let mut client = ProcessorClient::new(producer);

    let mut reader = BufReader::new(tokio::io::stdin()).lines();

    loop {
        tracing::info!("please enter a number:");

        let line = match reader.next_line().await.context("read line from stdin")? {
            Some(x) => x,
            None => {
                tracing::info!("exit");
                return Ok(());
            }
        };
        let num: u32 = line
            .as_str()
            .trim()
            .parse()
            .with_context(|| format!("number required, got {}", line))?;
        println!("pow({}) = {:?}", num, client.process(Num(num)).await);
    }
}

async fn run_consumer() -> anyhow::Result<()> {
    vc_processors::consumer::serve(
        listen_ready_message(ready_msg::<Num>()).await.context("write ready message")?,
        DefaultConsumer::new_xxhash(PowExecutor),
    )
    .await?;
    Ok(())
}
