use anyhow::Result;
use clap::{value_parser, Arg, ArgAction, ArgMatches, Command};

use damocles_worker_util::sealcalc;

mod csv;
mod tui;

pub const SUB_CMD_NAME: &str = "sealcalc";

pub(crate) fn subcommand<'a>() -> Command<'a> {
    Command::new(SUB_CMD_NAME)
        .arg(
            Arg::new("tree_d_mins")
                .long("tree_d_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify time consuming of tree_d stage, in minutes"),
        )
        .arg(
            Arg::new("tree_d_concurrent")
                .long("tree_d_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the maximum concurrent of tree_d"),
        )
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
                .help("Specify the maximum concurrent of pc1"),
        )
        .arg(
            Arg::new("pc2_mins")
                .long("pc2_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify time consuming of pc2 stage, in minutes"),
        )
        .arg(
            Arg::new("pc2_concurrent")
                .long("pc2_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the maximum concurrent of pc2"),
        )
        .arg(
            Arg::new("c2_mins")
                .long("c2_mins")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify time consuming of c2 stage, in minutes"),
        )
        .arg(
            Arg::new("c2_concurrent")
                .long("c2_concurrent")
                .takes_value(true)
                .required(true)
                .value_parser(value_parser!(usize))
                .help("Specify the maximum concurrent of c2"),
        )
        .arg(
            Arg::new("seed_mins")
                .long("seed_mins")
                .default_value("80")
                .value_parser(value_parser!(usize))
                .help("Specify time consuming of wait seed, in minutes"),
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
            Arg::new("calculate_days")
                .long("calculate_days")
                .default_value("30")
                .value_parser(value_parser!(usize))
                .help("Calculation time, in days"),
        )
        .arg(
            Arg::new("output_step")
                .long("output_step")
                .default_value("60")
                .value_parser(value_parser!(usize))
                .help("Output step size, in minutes. if this value is 60, each row will be separated by 1 hour"),
        )
        .arg(
            Arg::new("csv")
                .long("csv")
                .help("Output in CSV format")
                .action(ArgAction::SetTrue),
        )
}

pub(crate) fn submatch(subargs: &ArgMatches) -> Result<()> {
    let tree_d_mins: usize = *subargs.get_one("tree_d_mins").expect("required by clap");
    let pc1_mins: usize = *subargs.get_one("pc1_mins").expect("required by clap");
    let pc2_mins: usize = *subargs.get_one("pc2_mins").expect("required by clap");
    let c2_mins: usize = *subargs.get_one("c2_mins").expect("required by clap");
    let seed_mins: usize = *subargs.get_one("seed_mins").expect("required by clap");

    let tree_d_concurrent: usize = *subargs
        .get_one("tree_d_concurrent")
        .expect("required by clap");
    let pc1_concurrent: usize = *subargs.get_one("pc1_concurrent").expect("required by clap");
    let pc2_concurrent: usize = *subargs.get_one("pc2_concurrent").expect("required by clap");
    let c2_concurrent: usize = *subargs.get_one("c2_concurrent").expect("required by clap");

    let calculate_days: usize = *subargs.get_one("calculate_days").expect("required by clap");
    let output_step: usize = *subargs.get_one("output_step").expect("required by clap");

    let sealing_threads: usize = *subargs
        .get_one("sealing_threads")
        .expect("required by clap");

    let items = sealcalc::calc(
        (tree_d_mins, tree_d_concurrent),
        (pc1_mins, pc1_concurrent),
        (pc2_mins, pc2_concurrent),
        (c2_mins, c2_concurrent),
        seed_mins,
        sealing_threads,
        (calculate_days * 24 * 60, output_step),
    );
    let csv_mode = *subargs.get_one::<bool>("csv").unwrap_or(&false);

    if csv_mode {
        csv::display(
            &items,
            tree_d_concurrent,
            pc1_concurrent,
            pc2_concurrent,
            c2_concurrent,
            sealing_threads,
        )
    } else {
        tui::display(
            &items,
            tree_d_concurrent,
            pc1_concurrent,
            pc2_concurrent,
            c2_concurrent,
            sealing_threads,
        )
    }
}
