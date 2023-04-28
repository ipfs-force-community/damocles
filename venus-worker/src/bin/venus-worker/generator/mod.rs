use anyhow::Result;
use bytesize::ByteSize;
use clap::Parser;
use std::convert::TryFrom;
use std::path::PathBuf;
use tracing::{error, info};

use venus_worker::create_tree_d;

use venus_worker::{RegisteredSealProof, SealProof};

#[derive(Parser)]
pub(crate) enum GeneratorCommand {
    TreeD {
        /// The sector size
        #[arg(short = 's', long)]
        sector_size: ByteSize,
        /// Path to the static tree-d"
        #[arg(short = 'p', long)]
        path: PathBuf,
    },
}

pub(crate) fn run(cmd: &GeneratorCommand) -> Result<()> {
    match cmd {
        GeneratorCommand::TreeD { sector_size, path } => {
            let proof_type = SealProof::try_from(sector_size.as_u64())?;
            let registered_proof = RegisteredSealProof::from(proof_type);
            let cache_dir = PathBuf::from(&path);

            match create_tree_d(registered_proof, None, cache_dir) {
                Ok(_) => info!("generate static tree-d succeed"),
                Err(e) => error!("generate static tree-d {}", e),
            }

            Ok(())
        }
    }
}
