use std::fs::OpenOptions;
use std::io::Write;

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use clap::{value_t, values_t, App, Arg, SubCommand};
use tracing::{error, info};

use venus_worker::{logging, store::Store};

pub fn main() -> Result<()> {
    logging::init().expect("init logger");

    let store_init_cmd = SubCommand::with_name("init")
        .arg(
            Arg::with_name("location")
                .long("loc")
                .short("l")
                .multiple(true)
                .takes_value(true)
                .help("location of the store"),
        )
        .arg(
            Arg::with_name("capacity")
                .long("cap")
                .short("c")
                .takes_value(true)
                .help("reserved capacity for the store"),
        )
        .arg(
            Arg::with_name("output")
                .long("out")
                .short("o")
                .takes_value(true)
                .help("output path for store list"),
        );

    let matches = App::new("vc-storemgr")
        .version(env!("CARGO_PKG_VERSION"))
        .subcommand(store_init_cmd)
        .get_matches();

    match matches.subcommand() {
        ("init", Some(m)) => {
            let locs = values_t!(m, "location", String).context("get locations from flag")?;
            let cap_str = value_t!(m, "capacity", String).context("get capacity from flag")?;
            let cap = Byte::from_str(cap_str)?;

            let output = value_t!(m, "output", String).ok().map(|o| {
                OpenOptions::new()
                    .write(true)
                    .read(true)
                    .create(true)
                    .append(true)
                    .open(&o)
                    .map(|f| (o, f))
            });

            let mut output_file = match output {
                Some(Ok(f)) => Some(f),
                Some(Err(e)) => return Err(e.into()),
                None => None,
            };

            for loc in locs {
                if let Err(e) = Store::init(&loc, cap.get_bytes() as u64) {
                    error!(
                        loc = loc.as_str(),
                        err = logging::debug_field(&e),
                        "failed to init store"
                    )
                } else {
                    let _ = output_file.as_mut().map(|(p, f)| {
                        if let Err(e) = f.write_all(format!("{}\n", loc).as_bytes()) {
                            error!(
                                out = p.as_str(),
                                loc = loc.as_str(),
                                err = logging::debug_field(&e),
                                "failed to write into output file"
                            );
                        }
                    });
                    info!(loc = loc.as_str(), "store initialized");
                }
            }

            Ok(())
        }
        (other, _) => Err(anyhow!("unexpected subcommand {}", other)),
    }
}
