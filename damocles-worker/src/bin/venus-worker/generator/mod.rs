use anyhow::{anyhow, Result};
use byte_unit::Byte;
use clap::{value_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use std::convert::TryFrom;
use std::path::PathBuf;
use tracing::{error, info};

use damocles_worker::{create_tree_d, RegisteredSealProof, SealProof};

pub const SUB_CMD_NAME: &str = "generator";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let tree_d_cmd = SubCommand::with_name("tree-d")
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
        .subcommand(tree_d_cmd)
}

pub(crate) fn submatch(subargs: &ArgMatches<'_>) -> Result<()> {
    match subargs.subcommand() {
        ("tree-d", Some(m)) => {
            let size_str = value_t!(m, "sector-size", String)?;
            let size = Byte::from_str(size_str)?;
            let path = value_t!(m, "path", String)?;

            let proof_type = SealProof::try_from(size.get_bytes() as u64)?;
            let registered_proof = RegisteredSealProof::from(proof_type);
            let cache_dir = PathBuf::from(&path);

            match create_tree_d(registered_proof, None, cache_dir) {
                Ok(_) => info!("generate static tree-d succeed"),
                Err(e) => error!("generate static tree-d {}", e),
            }

            Ok(())
        }

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of generator", other)),
    }
}
