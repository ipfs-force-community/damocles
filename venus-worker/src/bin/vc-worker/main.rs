use anyhow::{anyhow, Result};
use byte_unit::Byte;
use clap::{value_t, App, Arg, SubCommand};

use venus_worker::{logging, start_deamon, start_mock};

mod processor;
mod store;

pub fn main() -> Result<()> {
    fil_logger::init();
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

    let app = App::new("vc-worker")
        .version(env!("CARGO_PKG_VERSION"))
        .subcommand(daemon_cmd)
        .subcommand(mock_cmd)
        .subcommand(processor_cmd)
        .subcommand(store_cmd);

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

        (name, _) => Err(anyhow!("unexpected subcommand `{}`", name)),
    }
}
