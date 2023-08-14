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
use std::io::{self, BufRead, BufReader};
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use serde::{Deserialize, Serialize};
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::core::{
    ext::{run_consumer, BoxedFinalizeHook, BoxedPrepareHook, ProducerBuilder},
    Processor, Task,
};

#[derive(Debug, Serialize, Deserialize)]
#[serde(transparent)]
struct Num(pub u32);

impl Task for Num {
    const STAGE: &'static str = "pow";
    type Output = Num;
}

#[derive(Copy, Clone, Default)]
struct PowProc;

impl Processor<Num> for PowProc {
    fn name(&self) -> String {
        "pow-d proc".to_string()
    }

    fn process(&self, task: Num) -> Result<<Num as Task>::Output> {
        if task.0 > 10086 {
            return Err(anyhow!("too large!!"));
        }

        Ok(Num(task.0.pow(2)))
    }
}

fn main() -> Result<()> {
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
        return run_consumer::<Num, PowProc>();
    };

    run_main()
}

fn run_main() -> Result<()> {
    let _span = warn_span!("parent", pid = std::process::id()).entered();
    let producer = ProducerBuilder::<BoxedPrepareHook<Num>, BoxedFinalizeHook<Num>>::new(
        current_exe().context("get current exe")?,
        vec!["sub".to_owned()],
    )
    .stable_timeout(Duration::from_secs(5))
    .spawn::<Num>()
    .context("build producer")?;

    info!(child = producer.child_pid(), "producer start");

    let stdin = io::stdin();
    let mut reader = BufReader::new(stdin);
    let mut line_buf = String::new();

    loop {
        info!("please enter a number:");

        line_buf.clear();

        let size = reader.read_line(&mut line_buf).context("read line from stdin")?;
        if size == 0 {
            info!("exit");
            return Ok(());
        }

        let num: u32 = line_buf
            .as_str()
            .trim()
            .parse()
            .with_context(|| format!("number required, got {:?}", line_buf))?;
        info!(num, "read in");

        match producer.process(Num(num)) {
            Ok(out) => {
                info!("get output: {:?}", out);
            }

            Err(e) => {
                warn!("get err: {:?}", e);
            }
        };
    }
}
