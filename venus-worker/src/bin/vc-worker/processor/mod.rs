use anyhow::{anyhow, Result};
use clap::{App, ArgMatches, SubCommand};

use venus_worker::{run_c2, run_pc2};

pub const SUB_CMD_NAME: &str = "processor";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let pc2_cmd = SubCommand::with_name("pc2");
    let c2_cmd = SubCommand::with_name("c2");
    SubCommand::with_name(SUB_CMD_NAME)
        .subcommand(pc2_cmd)
        .subcommand(c2_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        ("pc2", _) => run_pc2(),

        ("c2", _) => run_c2(),

        (other, _) => Err(anyhow!(
            "unexpected subcommand `{}` inside processor",
            other
        )),
    }
}
