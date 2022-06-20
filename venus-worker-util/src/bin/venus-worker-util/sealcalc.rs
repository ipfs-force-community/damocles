use anyhow::Result;
use clap::{value_parser, Arg, ArgAction, ArgMatches, Command};

use venus_worker_util::sealcalc;

mod csv;
mod tui;

pub const SUB_CMD_NAME: &str = "sealcalc";

pub(crate) fn subcommand<'a>() -> Command<'a> {
    Command::new(SUB_CMD_NAME)
        .arg(
            Arg::new("pc1_mins")
                .long("pc1_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify time consuming of pc1 stage, in minutes"),
        )
        .arg(
            Arg::new("pc1_concurrent")
                .long("pc1_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the concurrent of pc1"),
        )
        .arg(
            Arg::new("pc2_mins")
                .long("pc2_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("time consuming of pc2 stage, in minutes"),
        )
        .arg(
            Arg::new("pc2_concurrent")
                .long("pc2_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the concurrent of pc2"),
        )
        .arg(
            Arg::new("c2_mins")
                .long("c2_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("time consuming of c2 stage, in minutes"),
        )
        .arg(
            Arg::new("c2_concurrent")
                .long("c2_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the concurrent of c2"),
        )
        .arg(
            Arg::new("sealing_threads")
                .long("sealing_threads")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the number of sealing_threads"),
        )
        .arg(
            Arg::new("csv")
                .long("csv")
                .help("Show full CPU topology")
                .action(ArgAction::SetTrue),
        )
}

pub(crate) fn submatch(subargs: &ArgMatches) -> Result<()> {
    let pc1_mins: usize = *subargs.get_one("pc1_mins").expect("required by clap");
    let pc2_mins: usize = *subargs.get_one("pc2_mins").expect("required by clap");
    let c2_mins: usize = *subargs.get_one("c2_mins").expect("required by clap");

    let pc1_concurrent: usize = *subargs.get_one("pc1_concurrent").expect("required by clap");
    let pc2_concurrent: usize = *subargs.get_one("pc2_concurrent").expect("required by clap");
    let c2_concurrent: usize = *subargs.get_one("c2_concurrent").expect("required by clap");
    let sealing_threads: usize = *subargs
        .get_one("sealing_threads")
        .expect("required by clap");

    let total_minutes = 60 * 24 * 30;
    let items = sealcalc::calc(
        (pc1_mins, pc1_concurrent),
        (pc2_mins, pc2_concurrent),
        (c2_mins, c2_concurrent),
        sealing_threads,
        total_minutes,
    );
    let csv_mode = *subargs.get_one::<bool>("csv").unwrap_or(&false);

    if csv_mode {
        csv::display(
            &items,
            pc1_concurrent,
            pc2_concurrent,
            c2_concurrent,
            sealing_threads,
        )
    } else {
        tui::display(
            &items,
            pc1_concurrent,
            pc2_concurrent,
            c2_concurrent,
            sealing_threads,
        )
    }
}
