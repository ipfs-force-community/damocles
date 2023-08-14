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

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use tracing::info;
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::core::{ext::run_consumer, Processor, Task};

#[derive(Debug, Serialize, Deserialize)]
#[serde(transparent)]
struct Num(pub u32);

impl Task for Num {
    const STAGE: &'static str = "sqr";
    type Output = Num;
}

#[derive(Copy, Clone, Default)]
struct PowProc;

impl Processor<Num> for PowProc {
    fn name(&self) -> String {
        "pow-d proc".to_string()
    }

    fn process(&self, task: Num) -> Result<<Num as Task>::Output> {
        Ok(Num(task.0.pow(2)))
    }
}

fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(fmt::layer())
        .with(
            EnvFilter::builder()
                .with_default_directive(LevelFilter::DEBUG.into())
                .from_env()
                .context("env filter")?,
        )
        .init();

    info!("start sqr consumer");
    run_consumer::<Num, PowProc>()
}
