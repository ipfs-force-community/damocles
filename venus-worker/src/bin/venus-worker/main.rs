use std::path::PathBuf;

use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use tokio::runtime::Builder;

use venus_worker::{logging, set_panic_hook, start_daemon};

mod generator;
mod processor;
mod store;
mod worker;

#[derive(Parser)]
#[command(about, long_about, version = &**venus_worker::VERSION)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Run the venus-worker daemon
    Daemon {
        /// Path to the config file
        #[arg(short, long)]
        config: PathBuf,
    },
    #[command(subcommand)]
    Generator(generator::GeneratorCommand),
    #[command(subcommand)]
    Processor(processor::ProcessorCommand),
    #[command(subcommand)]
    Store(store::StoreCommand),
    Worker(worker::WorkerCommand),
}

pub fn main() -> Result<()> {
    let rt = Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("construct tokio runtime")?;

    let _rt_guard = rt.enter();

    logging::init()?;
    set_panic_hook(true);

    let cli = Cli::parse();
    match cli.command {
        Commands::Daemon { config } => start_daemon(config),
        Commands::Generator(cmd) => generator::run(&cmd),
        Commands::Processor(cmd) => processor::run(&cmd),
        Commands::Store(cmd) => store::run(&cmd),
        Commands::Worker(cmd) => worker::run(&cmd),
    }
}
