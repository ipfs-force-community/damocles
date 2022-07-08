use anyhow::{anyhow, Context, Result};
use clap::{values_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use tracing::{error, info};

use venus_worker::{objstore::filestore::FileStore, store::Store};

#[cfg(target_os = "linux")]
mod shm;

pub const SUB_CMD_NAME: &str = "store";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let store_init_cmd = SubCommand::with_name("sealing-init").arg(
        Arg::with_name("location")
            .long("loc")
            .short("l")
            .multiple(true)
            .takes_value(true)
            .help("location of the store"),
    );

    let filestore_init_cmd = SubCommand::with_name("file-init").arg(
        Arg::with_name("location")
            .long("loc")
            .short("l")
            .multiple(true)
            .takes_value(true)
            .help("location of the store"),
    );

    let shm_init_cmd = SubCommand::with_name("shm-init").args(&[
        Arg::with_name("numa_node_index")
            .long("node")
            .short("n")
            .required(true)
            .takes_value(true)
            .help("Specify the numa node"),
        Arg::with_name("size")
            .long("size")
            .short("s")
            .required(true)
            .takes_value(true)
            .possible_values(&["32GiB", "64GiB"])
            .help("Specify the size of each shm file. (e.g., 1B, 2KB, 3kiB, 1MB, 2MiB, 3GB, 1GiB, ...)"),
        Arg::with_name("number_of_files")
            .long("num")
            .short("c")
            .required(true)
            .takes_value(true)
            .help("Specify the number of shm files to be created"),
        Arg::with_name("shm_numa_dir_pattern")
            .long("pat")
            .short("p")
            .required(false)
            .default_value(default_shm_numa_dir_pattern())
            .help("Specify the shared memory directory pattern"),
    ]);

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(store_init_cmd)
        .subcommand(filestore_init_cmd)
        .subcommand(shm_init_cmd)
}

pub(crate) fn submatch(subargs: &ArgMatches<'_>) -> Result<()> {
    match subargs.subcommand() {
        ("sealing-init", Some(m)) => {
            let locs = values_t!(m, "location", String).context("get locations from flag")?;

            for loc in locs {
                match Store::init(&loc) {
                    Ok(l) => info!(loc = ?l, "store initialized"),
                    Err(e) => error!(
                        loc = loc.as_str(),
                        err = ?e,
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }

        ("file-init", Some(m)) => {
            let locs = values_t!(m, "location", String).context("get locations from flag")?;

            for loc in locs {
                match FileStore::init(&loc) {
                    Ok(_) => info!(?loc, "store initialized"),
                    Err(e) => error!(
                        loc = loc.as_str(),
                        err = ?e,
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }
        ("shm-init", Some(m)) => shm_init(m),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of store", other)),
    }
}

#[cfg(target_os = "linux")]
fn shm_init(m: &ArgMatches) -> Result<()> {
    use clap::value_t;

    let numa_node_idx = value_t!(m, "numa_node_index", u32).context("invalid NUMA node index")?;
    let size = value_t!(m, "size", bytesize::ByteSize).context("invalid file size")?;
    let num = value_t!(m, "number_of_files", usize).context("invalid number_of_files")?;
    let pat = value_t!(m, "shm_numa_dir_pattern", String)?;
    let files = shm::init_shm_files(numa_node_idx, size, num, pat)?;
    println!("Created SHM files:");
    for file in files {
        println!("{}", file.display())
    }
    Ok(())
}

#[cfg(not(target_os = "linux"))]
fn shm_init(_m: &ArgMatches) -> Result<()> {
    Err(anyhow!("This command is only supported for the Linux operating system"))
}

fn default_shm_numa_dir_pattern() -> &'static str {
    use std::sync::Once;

    use vc_processors::fil_proofs::settings::ShmNumaDirPattern;

    static mut PAT: String = String::new();
    static INIT: Once = Once::new();

    INIT.call_once(|| unsafe { PAT = format!("filecoin-proof-label/numa_{}", ShmNumaDirPattern::NUMA_NODE_IDX_VAR_NAME) });

    unsafe { PAT.as_str() }
}
