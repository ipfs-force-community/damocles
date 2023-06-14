use anyhow::{anyhow, Result};
use clap::Command;

mod hwinfo;
mod sealcalc;

fn main() -> Result<()> {
    let ver_string = format!(
        "v{}-{}",
        env!("CARGO_PKG_VERSION"),
        option_env!("GIT_COMMIT").unwrap_or("dev")
    );

    let matches = Command::new("damocles-worker-util")
        .version(ver_string.as_str())
        .about("damocles-worker utility collection")
        .arg_required_else_help(true)
        .subcommand(hwinfo::subcommand())
        .subcommand(sealcalc::subcommand())
        .get_matches();

    match matches.subcommand() {
        Some((hwinfo::SUB_CMD_NAME, args)) => hwinfo::submatch(args),
        Some((sealcalc::SUB_CMD_NAME, args)) => sealcalc::submatch(args),
        _ => Err(anyhow!("unexpected subcommand")),
    }
}
