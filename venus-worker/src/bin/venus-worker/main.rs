use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use clap::{value_t, App, AppSettings, Arg, SubCommand};
use tokio::runtime::Builder;

use venus_worker::{logging, start_deamon, start_mock};

mod processor;
mod store;
mod worker;
mod generator;

pub fn main() -> Result<()> {
    let rt = Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("construct tokio runtime")?;

    let _rt_guart = rt.enter();

    logging::init()?;

    let mock_cmd = SubCommand::with_name("mock")
        .arg(
            Arg::with_name("miner")
                .long("miner")
                .short("m")
                .takes_value(true)
                .help("miner actor id for mock server"),
        )
        .arg(
            Arg::with_name("sector-size")
                .long("sector-size")
                .short("s")
                .takes_value(true)
                .help("sector size for mock server"),
        )
        .arg(
            Arg::with_name("config")
                .long("config")
                .short("c")
                .takes_value(true)
                .help("path for the config file"),
        );

    let daemon_cmd = SubCommand::with_name("daemon")
        .arg(
            Arg::with_name("api")
                .long("api")
                .takes_value(true)
                .help("sealer api addr"),
        )
        .arg(
            Arg::with_name("config")
                .long("config")
                .short("c")
                .takes_value(true)
                .help("path for the config file"),
        );

    let processor_cmd = processor::subcommand();
    let store_cmd = store::subcommand();
    let worker_cmd = worker::subcommand();
    let generator_cmd = generator::subcommand();

    let app = App::new("vc-worker")
        .version(env!("CARGO_PKG_VERSION"))
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(daemon_cmd)
        .subcommand(mock_cmd)
        .subcommand(processor_cmd)
        .subcommand(store_cmd)
        .subcommand(worker_cmd)
        .subcommand(generator_cmd);

    let matches = app.get_matches();

    match matches.subcommand() {
        ("mock", Some(m)) => {
            let miner = value_t!(m, "miner", u64)?;
            let size_str = value_t!(m, "sector-size", String)?;
            let size = Byte::from_str(size_str)?;
            let cfg_path = value_t!(m, "config", String)?;

            start_mock(miner, size.get_bytes() as u64, cfg_path)
        }

        ("daemon", Some(m)) => {
            let cfg_path = value_t!(m, "config", String)?;

            start_deamon(cfg_path)
        }

        (processor::SUB_CMD_NAME, Some(args)) => processor::submatch(args),

        (store::SUB_CMD_NAME, Some(args)) => store::submatch(args),

        (worker::SUB_CMD_NAME, Some(args)) => worker::submatch(args),

        (generator::SUB_CMD_NAME, Some(args)) => generator::submatch(args),

        (name, _) => Err(anyhow!("unexpected subcommand `{}`", name)),
    }
}
