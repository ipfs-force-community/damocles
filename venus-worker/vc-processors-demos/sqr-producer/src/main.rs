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
    let mut producer = ProducerBuilder::<BoxedPrepareHook<Num>, BoxedFinalizeHook<Num>>::new(
        current_exe().context("get current exe")?,
        vec!["sub".to_owned()],
    )
    .stable_timeout(Duration::from_secs(5))
    .build::<Num>()
    .context("build producer")?;

    producer.start_response_handler().context("start response handler")?;

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
            .with_context(|| format!("number requried, got {:?}", line_buf))?;
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
