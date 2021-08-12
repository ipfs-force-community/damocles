use anyhow::{Context, Result};
use clap::{value_t, App, Arg, ArgMatches, SubCommand};

use venus_worker::{
    client::{connect, WorkerClient},
    Config,
};

pub const SUB_CMD_NAME: &str = "worker";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let list_cmd = SubCommand::with_name("list");

    SubCommand::with_name(SUB_CMD_NAME)
        .arg(
            Arg::with_name("config")
                .long("config")
                .short("c")
                .takes_value(true)
                .required(true)
                .help("path to the config file"),
        )
        .subcommand(list_cmd)
}

pub fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    unimplemented!();
}

fn get_client<'a>(m: &ArgMatches<'a>) -> Result<WorkerClient> {
    let cfg_path = value_t!(m, "config", String).context("get config path")?;
    let cfg = Config::load(&cfg_path)?;
    connect(&cfg)
}
