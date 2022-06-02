use anyhow::{anyhow, Result};
use clap::App;
mod cpu;
mod disk;
mod mem;

fn main() -> Result<()> {
    let app = App::new("hwinfo")
        .subcommand(cpu::subcommand())
        .subcommand(disk::subcommand())
        .subcommand(mem::subcommand());

    let matches = app.get_matches();

    match matches.subcommand() {
        (cpu::SUB_CMD_NAME, Some(args)) => cpu::submatch(args),
        (disk::SUB_CMD_NAME, Some(args)) => disk::submatch(args),
        (mem::SUB_CMD_NAME, Some(args)) => mem::submatch(args),
        (name, _) => Err(anyhow!("unexpected subcommand `{}`", name)),
    }
}
