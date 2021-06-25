use anyhow::{anyhow, Context, Result};
use clap::{values_t, App, Arg, SubCommand};
use tracing::{error, info};

use venus_worker::{logging, sealing::store::Store};

pub fn main() -> Result<()> {
    logging::init().expect("init logger");

    let store_init_cmd = SubCommand::with_name("init").arg(
        Arg::with_name("location")
            .long("loc")
            .short("l")
            .multiple(true)
            .takes_value(true)
            .help("location of the store"),
    );

    let matches = App::new("vc-storemgr")
        .version(env!("CARGO_PKG_VERSION"))
        .subcommand(store_init_cmd)
        .get_matches();

    match matches.subcommand() {
        ("init", Some(m)) => {
            let locs = values_t!(m, "location", String).context("get locations from flag")?;

            for loc in locs {
                match Store::init(&loc) {
                    Ok(l) => info!(loc = logging::debug_field(&l), "store initialized"),
                    Err(e) => error!(
                        loc = loc.as_str(),
                        err = logging::debug_field(&e),
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }
        (other, _) => Err(anyhow!("unexpected subcommand {}", other)),
    }
}
