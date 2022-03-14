use anyhow::{anyhow, Result};
use byte_unit::Byte;
use clap::{value_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use tracing::{error, info};

use venus_worker::generate_static_tree_d;

pub const SUB_CMD_NAME: &str = "generator";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let pieces_cmd = SubCommand::with_name("tree-d")
        .arg(
            Arg::with_name("sector-size")
                .long("sector-size")
                .short("s")
                .takes_value(true)
                .help("sector size"),
        )
        .arg(
            Arg::with_name("path")
                .long("path")
                .short("p")
                .takes_value(true)
                .help("path for the staic-tree-d"),
        );

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(pieces_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        ("tree-d", Some(m)) => {
            let size_str = value_t!(m, "sector-size", String)?;
            let size = Byte::from_str(size_str)?;
            let path = value_t!(m, "path", String)?;

            match generate_static_tree_d(size.get_bytes() as u64, path) {
                Ok(_) => info!("generate static tree-d succeed"),
                Err(e) => error!("generate static tree-d {}", e),
            }

            Ok(())
        }

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of generator", other)),
    }
}
