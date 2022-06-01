use std::env::{self, current_exe};
use std::fs::{create_dir_all, remove_dir_all, OpenOptions};
use std::io::{self, BufRead, BufReader};
use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use crossbeam_channel::bounded;
use tracing::{info, warn, warn_span};
use tracing_subscriber::{filter::LevelFilter, fmt, prelude::*, EnvFilter};
use vc_processors::{
    builtin::{processors::BuiltinProcessor, tasks::TreeD},
    core::{
        ext::{run_consumer, ProducerBuilder, Request},
        Processor,
    },
    fil_proofs::RegisteredSealProof,
};

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
        return run_consumer::<TreeD, BuiltinProcessor>();
    };

    run_main()
}

fn run_main() -> Result<()> {
    // act as a simple concurrent & rate limiter
    let (limit_tx, limit_rx) = bounded(1);

    std::thread::spawn(move || loop {
        std::thread::sleep(Duration::from_secs(5));
        if limit_rx.try_recv().is_ok() {
            info!("re-fill one token");
        }
    });

    let _span = warn_span!("parent", pid = std::process::id()).entered();
    let mut producer = ProducerBuilder::<_, _>::new(current_exe().context("get current exe")?, vec!["sub".to_owned()])
        .stable_timeout(Duration::from_secs(5))
        .hook_prepare(move |_: &Request<TreeD>| -> Result<()> {
            let _ = limit_tx.send(());
            info!("token acquired");
            Ok(())
        })
        .hook_finalize(move |_: &Request<TreeD>| {
            info!("do nothing");
        })
        .build::<TreeD>()
        .context("build producer")?;

    producer.start_response_handler().context("start response handler")?;

    info!(child = producer.child_pid(), "producer start");

    let stdin = io::stdin();
    let mut reader = BufReader::new(stdin);
    let mut line_buf = String::new();

    loop {
        info!("please enter a dir:");
        line_buf.clear();

        let size = reader.read_line(&mut line_buf).context("read line from stdin")?;
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
        info!(
            ?dir,
            "will generate an empty staged file of 8MiB & a tree_d file, and clean them up after"
        );

        let staged_file_path = dir.join("staged");
        create_dir_all(&dir).with_context(|| format!("create dir at {:?}", &dir))?;
        let fs = OpenOptions::new()
            .create(true)
            .truncate(true)
            .read(true)
            .write(true)
            .open(&staged_file_path)
            .context("create staged file")?;

        fs.set_len(2 << 10).context("set len for staged file")?;

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

        remove_dir_all(dir).context("remove the demo dir")?;
    }
}
