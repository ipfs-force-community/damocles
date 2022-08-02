use anyhow::{anyhow, Context, Result};
use clap::{values_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use tracing::{error, info};

use venus_worker::{objstore::filestore::FileStore, store::Store};

#[cfg(target_os = "linux")]
mod hugepage;

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

    let hugepage_file_init_cmd = SubCommand::with_name("hugepage-file-init")
        .arg(
            Arg::with_name("numa_node_index")
                .long("node")
                .short("n")
                .required(true)
                .takes_value(true)
                .help("Specify the numa node"),
        )
        .arg(
            Arg::with_name("size")
                .long("size")
                .short("s")
                .required(true)
                .takes_value(true)
                .help("Specify the size of each hugepage memory file. (e.g., 1B, 2KB, 3kiB, 1MB, 2MiB, 3GB, 1GiB, ...)"),
        )
        .arg(
            Arg::with_name("number_of_files")
                .long("num")
                .short("c")
                .required(true)
                .takes_value(true)
                .help("Specify the number of hugepage memory files to be created"),
        )
        .arg(Arg::with_name("path").long("path").required_unless("path_pattern").takes_value(true).long_help(
            "Specify the path to the output hugepage memory files and using the default pattern (/specified_hugepage_file_path/numa_$NUMA_NODE_INDEX).
The created files looks like this:
/specified_hugepage_file_path/numa_0/file
/specified_hugepage_file_path/numa_1/file
/specified_hugepage_file_path/numa_2/file
...

This argument will be ignored if `path_pattern` is specified.",
        ))
        .arg(
            Arg::with_name("path_pattern")
                .long("path_pattern")
                .required_unless("path")
               . takes_value(true)
                .long_help(
                    "Specify the path pattern for the output hugepage memory files where $NUMA_NODE_INDEX represents 
the numa node index placeholder, which extracts the number in the folder name as the numa node index.

If both the argument `path` and the argument `path_pattern` are specified, the argument `path` will be ignored.",
                ),
        );

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(store_init_cmd)
        .subcommand(filestore_init_cmd)
        .subcommand(hugepage_file_init_cmd)
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
        ("hugepage-file-init", Some(m)) => hugepage_file_init(m),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of store", other)),
    }
}

#[cfg(target_os = "linux")]
fn hugepage_file_init(m: &ArgMatches) -> Result<()> {
    use clap::value_t;
    use venus_worker::seal_util::MemoryFileDirPattern;

    let numa_node_idx = value_t!(m, "numa_node_index", u32).context("invalid NUMA node index")?;
    let size = value_t!(m, "size", bytesize::ByteSize).context("invalid file size")?;
    let num = value_t!(m, "number_of_files", usize).context("invalid number_of_files")?;

    let pattern = match (
        m.value_of("path").map(MemoryFileDirPattern::new_default),
        m.value_of("path_pattern").map(MemoryFileDirPattern::without_prefix),
    ) {
        (Some(_), Some(p)) | (Some(p), None) | (None, Some(p)) => p,
        (None, None) => unreachable!("Unreachable duo to clap require_unless"),
    };

    let files = hugepage::create_hugepage_mem_files(numa_node_idx, size, num, pattern.to_path(numa_node_idx))?;
    println!("Created hugepage memory files:");
    for file in files {
        println!("{}", file.display())
    }
    Ok(())
}

#[cfg(not(target_os = "linux"))]
fn hugepage_file_init(_m: &ArgMatches) -> Result<()> {
    Err(anyhow!("This command is only supported for the Linux operating system"))
}
