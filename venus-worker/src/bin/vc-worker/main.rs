use anyhow::{anyhow, Result};
use byte_unit::Byte;
use clap::{value_t, App, Arg, SubCommand};

use venus_worker::logging;

mod mock;

pub fn main() -> Result<()> {
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
            Arg::with_name("store-list")
                .long("store-list")
                .short("l")
                .takes_value(true)
                .help("store list file path"),
        )
        .arg(
            Arg::with_name("remote-store")
                .long("remote-store")
                .short("r")
                .takes_value(true)
                .help("remote store path"),
        );

    let matches = App::new("vc-worker")
        .version(env!("CARGO_PKG_VERSION"))
        .subcommand(mock_cmd)
        .get_matches();

    match matches.subcommand() {
        ("mock", Some(m)) => {
            let miner = value_t!(m, "miner", u64)?;
            let size_str = value_t!(m, "sector-size", String)?;
            let size = Byte::from_str(size_str)?;
            let store_list = value_t!(m, "store-list", String)?;
            let remote_store = value_t!(m, "remote-store", String)?;

            mock::start_mock(miner, size.get_bytes() as u64, store_list, remote_store)
        }

        (other, _) => Err(anyhow!("unexpected subcommand {}", other)),
    }
}
