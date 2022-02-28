use anyhow::{anyhow, Context, Result};
use clap::{values_t, App, Arg, ArgMatches,AppSettings, SubCommand};
use tracing::{error, info};

use venus_worker::{logging, objstore::filestore::FileStore, store::Store};

pub const SUB_CMD_NAME: &str = "store";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let store_init_cmd = SubCommand::with_name("sealing-init").arg(
        Arg::with_name("location")
            .long("loc")
            .short("l")
            .multiple(true)
            .takes_value(true)
            .help("location of the store"),
    );

    let filestore_init_cmd = SubCommand::with_name("file-init").arg(
        Arg::with_name("location")
            .long("loc")
            .short("l")
            .multiple(true)
            .takes_value(true)
            .help("location of the store"),
    );

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(store_init_cmd)
        .subcommand(filestore_init_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        ("sealing-init", Some(m)) => {
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

        ("file-init", Some(m)) => {
            let locs = values_t!(m, "location", String).context("get locations from flag")?;

            for loc in locs {
                match FileStore::init(&loc) {
                    Ok(_) => info!(loc = logging::debug_field(&loc), "store initialized"),
                    Err(e) => error!(
                        loc = loc.as_str(),
                        err = logging::debug_field(&e),
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of store", other)),
    }
}
