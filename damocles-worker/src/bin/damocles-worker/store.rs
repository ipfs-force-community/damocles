use std::path::PathBuf;

use anyhow::Result;
use bytesize::ByteSize;
use clap::Parser;
use tracing::{error, info};

use damocles_worker::seal_util::MemoryFileDirPattern;
use damocles_worker::{objstore::filestore::FileStore, store::Store};

#[cfg(target_os = "linux")]
mod hugepage;

#[allow(clippy::enum_variant_names)]
#[derive(Parser)]
pub(crate) enum StoreCommand {
    /// Initializing the sealing directory
    SealingInit {
        /// Location of the store
        #[arg(short = 'l', long = "loc", num_args=1..)]
        location: Vec<PathBuf>,
    },
    /// Initializing the persistence store directory
    FileInit {
        /// Location of the store
        #[arg(short = 'l', long = "loc", num_args=1..)]
        location: Vec<PathBuf>,
    },
    HugepageFileInit {
        /// Specify the numa node
        #[arg(short = 'n', long = "node", alias = "numa_node_index")]
        numa_node_index: u32,
        /// Specify the size of each hugepage memory file. (e.g., 1B, 2KB, 3kiB, 1MB, 2MiB, 3GB, 1GiB, ...)
        #[arg(short = 's', long)]
        size: bytesize::ByteSize,
        /// Specify the number of hugepage memory files to be created
        #[arg(short = 'c', long = "num", alias = "number_of_files")]
        number_of_files: usize,
        /// Specify the path to the output hugepage memory files and using the default pattern (/specified_hugepage_file_path/numa_$NUMA_NODE_INDEX).
        /// The created files looks like this:
        /// /specified_hugepage_file_path/numa_0/file
        /// /specified_hugepage_file_path/numa_1/file
        /// /specified_hugepage_file_path/numa_2/file
        /// ...
        ///
        /// This argument will be ignored if `path_pattern` is specified.
        #[arg(long, required_unless_present("path_pattern"))]
        path: Option<String>,
        /// Specify the path pattern for the output hugepage memory files where $NUMA_NODE_INDEX represents
        /// the numa node index placeholder, which extracts the number in the folder name as the numa node index.
        ///
        /// If both the argument `path` and the argument `path_pattern` are specified, the argument `path` will be ignored.
        #[arg(long, alias = "path_pattern", required_unless_present("path"))]
        path_pattern: Option<String>,
    },
}

pub(crate) fn run(cmd: &StoreCommand) -> Result<()> {
    match cmd {
        StoreCommand::SealingInit { location } => {
            for loc in location {
                match Store::init(loc, true) {
                    Ok(l) => info!(loc = ?l, "store initialized"),
                    Err(e) => error!(
                        loc = ?loc.display(),
                        err = ?e,
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }
        StoreCommand::FileInit { location } => {
            for loc in location {
                match FileStore::init(loc) {
                    Ok(_) => info!(?loc, "store initialized"),
                    Err(e) => error!(
                        loc = ?loc.display(),
                        err = ?e,
                        "failed to init store"
                    ),
                }
            }

            Ok(())
        }
        StoreCommand::HugepageFileInit {
            numa_node_index,
            size,
            number_of_files,
            path,
            path_pattern,
        } => {
            let pattern = match (
                path.as_ref().map(MemoryFileDirPattern::new_default),
                path_pattern
                    .as_ref()
                    .map(MemoryFileDirPattern::without_prefix),
            ) {
                (Some(_), Some(p)) | (Some(p), None) | (None, Some(p)) => p,
                (None, None) => unreachable!(
                    "Unreachable duo to clap `required_unless_present`"
                ),
            };
            hugepage_file_init(
                *numa_node_index,
                *size,
                *number_of_files,
                pattern,
            )
        }
    }
}

#[cfg(target_os = "linux")]
fn hugepage_file_init(
    numa_node_idx: u32,
    size: ByteSize,
    count: usize,
    pat: MemoryFileDirPattern,
) -> Result<()> {
    let files = hugepage::create_hugepage_mem_files(
        numa_node_idx,
        size,
        count,
        pat.to_path(numa_node_idx),
    )?;
    println!("Created hugepage memory files:");
    for file in files {
        println!("{}", file.display())
    }
    Ok(())
}

#[cfg(not(target_os = "linux"))]
fn hugepage_file_init(
    _numa_node_idx: u32,
    _size: ByteSize,
    _count: usize,
    _pat: MemoryFileDirPattern,
) -> Result<()> {
    Err(anyhow::anyhow!(
        "This command is only supported for the Linux operating system"
    ))
}
