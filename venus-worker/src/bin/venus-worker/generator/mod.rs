use anyhow::{anyhow, Result};
use byte_unit::Byte;
use clap::{value_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use tracing::{error, info};

use venus_worker::{generate_piece};

pub const SUB_CMD_NAME: &str = "generator";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let pieces_cmd = SubCommand::with_name("pieces")
        .arg(
            Arg::with_name("sector-size")
                .long("sector-size")
                .short("s")
                .takes_value(true)
                .help("sector size"),
        )
        .arg(
            Arg::with_name("unsealed-path")
                .long("unsealed-path")
                .short("u")
                .takes_value(true)
                .help("path for the unsealed file"),
        )
        .arg(
            Arg::with_name("pieces-path")
                .long("pieces-path")
                .short("p")
                .takes_value(true)
                .help("path for the pieces info file"),
        );

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(pieces_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        ("pieces", Some(m)) => {
            let size_str = value_t!(m, "sector-size", String)?;
            let size = Byte::from_str(size_str)?;
            let unsealed_path = value_t!(m, "unsealed-path", String)?;
            let pieces_path = value_t!(m, "pieces-path", String)?;

            match generate_piece(size.get_bytes() as u64, unsealed_path, pieces_path) {
                Ok(_) => info!("generate pieces succeed"),
                Err(e) => error!(
                    "failed to generate pieces {}", e
                ),
            }

            Ok(())
        }

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of generator", other)),
    }
}
