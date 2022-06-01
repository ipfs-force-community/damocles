use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use tracing::info;
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::core::{ext::run_consumer, Processor, Task};

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
