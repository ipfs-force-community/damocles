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
use std::io::{self, BufRead, BufReader};
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::{
    builtin::tasks::TreeD,
    core::{
        ext::{run_consumer, ProducerBuilder},
        Processor, Task,
    },
    fil_proofs::RegisteredSealProof,
};

#[derive(Clone, Copy, Default)]
struct TreeDProc;

impl Processor<TreeD> for TreeDProc {
    fn name(&self) -> String {
        "tree-d proc".to_string()
    }

    fn process(&self, task: TreeD) -> Result<<TreeD as Task>::Output> {
        info!(dir = ?task.cache_dir, "process tree_d task");
        std::thread::sleep(Duration::from_secs(3));
        Ok(Default::default())
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
        return run_consumer::<TreeD, TreeDProc>();
    };

    run_main()
}

fn run_main() -> Result<()> {
    let _span = warn_span!("parent", pid = std::process::id()).entered();
    let producer = ProducerBuilder::new(
        current_exe().context("get current exe")?,
        vec!["sub".to_owned()],
    )
    .stable_timeout(Duration::from_secs(5))
    .spawn::<TreeD>()
    .context("build producer")?;

    info!(child = producer.child_pid(), "producer start");

    let stdin = io::stdin();
    let mut reader = BufReader::new(stdin);
    let mut line_buf = String::new();

    loop {
        info!("please enter a dir:");
        line_buf.clear();

        let size = reader
            .read_line(&mut line_buf)
            .context("read line from stdin")?;
        if size == 0 {
            info!("exit");
            return Ok(());
        }

        let loc = line_buf.as_str().trim();
        if loc.is_empty() {
            info!("get empty location");
            return Ok(());
        }

        let dir = PathBuf::from(loc);
        let staged_file_path = dir.join("staged");

        match producer.process(TreeD {
            registered_proof: RegisteredSealProof::StackedDrg2KiBV1_1,
            staged_file: staged_file_path,
            cache_dir: dir.clone(),
        }) {
            Ok(out) => {
                info!("get output: {:?}", out);
            }

            Err(e) => {
                warn!("get err: {:?}", e);
            }
        };
    }
}
