use anyhow::Result;
use clap::{App, ArgMatches, SubCommand};

pub const SUB_CMD_NAME: &str = "hwinfo";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    SubCommand::with_name(SUB_CMD_NAME).help_message("Show hardware infomation")
}

pub fn submatch(_subargs: &ArgMatches<'_>) -> Result<()> {
    Ok(())
}
