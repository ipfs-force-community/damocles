use anyhow::{Context, Result};
use clap::Subcommand;
use vc_processors::{
    builtin::{
        processors::{BuiltinProcessor, TransferProcessor},
        tasks::{
            AddPieces, SnapEncode, SnapProve, Transfer, TreeD, WindowPoSt, WinningPoSt, C2, PC1, PC2, STAGE_NAME_ADD_PIECES, STAGE_NAME_C2,
            STAGE_NAME_PC1, STAGE_NAME_PC2, STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
            STAGE_NAME_WINDOW_POST, STAGE_NAME_WINNING_POST,
        },
    },
    core::ext::{run_consumer, run_consumer_with_proc},
};

const C2_ABOUT: &str = "damocles-worker built-in c2";

#[derive(Subcommand)]
pub enum ProcessorCommand {
    #[command(name=STAGE_NAME_ADD_PIECES)]
    AddPieces,
    #[command(name=STAGE_NAME_TREED)]
    TreeD,
    #[command(name=STAGE_NAME_PC1)]
    PC1 {
        /// Specify the path to the hugepage memory file and scan the hugepage memory files
        /// using the default pattern (/specified_hugepage_file_path/numa_$NUMA_NODE_INDEX).
        /// It will match:
        /// /specified_hugepage_file_path/numa_0/any_files
        /// /specified_hugepage_file_path/numa_1/any_files
        /// /specified_hugepage_file_path/numa_2/any_files
        /// ...
        ///
        /// Make sure that the memory files stored in the folder are created in the numa node corresponding to $NUMA_NODE_INDEX.
        /// This argument will be ignored if `hugepage_files_path_pattern` is specified.
        #[arg(long, alias = "hugepage_files_path", env = "HUGEPAGE_FILES_PATH")]
        hugepage_files_path: Option<String>,
        /// Specify the hugepage memory file path pattern where $NUMA_NODE_INDEX represents
        /// the numa node index placeholder, which extracts the number in the folder name as the numa node index
        /// Make sure that the memory files stored in the folder are created in the numa node corresponding to $NUMA_NODE_INDEX.
        /// If both the argument `hugepage_files_path` and the argument `hugepage_files_path_pattern` are specified,
        /// the argument `hugepage_files_path` will be ignored.
        #[arg(long, alias = "hugepage_files_path_pattern", env = "HUGEPAGE_FILES_PATH_PATTERN")]
        hugepage_files_path_pattern: Option<String>,
    },
    #[command(name=STAGE_NAME_PC2)]
    PC2,
    #[command(name=STAGE_NAME_C2, about=C2_ABOUT)]
    C2,
    #[command(name=STAGE_NAME_SNAP_ENCODE)]
    SnapEncode,
    #[command(name=STAGE_NAME_SNAP_PROVE)]
    SnapProve,
    #[command(name=STAGE_NAME_TRANSFER)]
    Transfer {
        #[arg(long, alias = "disable_link", env = "DISABLE_LINK")]
        disable_link: bool,
    },
    #[command(name=STAGE_NAME_WINDOW_POST)]
    WindowPoSt,
    #[command(name=STAGE_NAME_WINNING_POST)]
    WinningPoSt,
}

pub(crate) fn run(cmd: &ProcessorCommand) -> Result<()> {
    match cmd {
        ProcessorCommand::AddPieces => run_consumer::<AddPieces, BuiltinProcessor>(),
        ProcessorCommand::TreeD => run_consumer::<TreeD, BuiltinProcessor>(),
        ProcessorCommand::PC1 {
            hugepage_files_path,
            hugepage_files_path_pattern,
        } => {
            use damocles_worker::seal_util::{scan_memory_files, MemoryFileDirPattern};
            use storage_proofs_porep::stacked::init_numa_mem_pool;

            // Argument `hugepage_files_path_pattern` take precedence over argument `hugepage_files_path`
            match (
                hugepage_files_path.as_ref().map(MemoryFileDirPattern::new_default),
                hugepage_files_path_pattern.as_ref().map(MemoryFileDirPattern::without_prefix),
            ) {
                (Some(_), Some(p)) | (Some(p), None) | (None, Some(p)) => {
                    init_numa_mem_pool(scan_memory_files(&p).context("scan_memory_files")?);
                }
                (None, None) => {}
            }

            run_consumer::<PC1, BuiltinProcessor>()
        }
        ProcessorCommand::PC2 => run_consumer::<PC2, BuiltinProcessor>(),
        ProcessorCommand::C2 => run_consumer::<C2, BuiltinProcessor>(),
        ProcessorCommand::SnapEncode => run_consumer::<SnapEncode, BuiltinProcessor>(),
        ProcessorCommand::SnapProve => run_consumer::<SnapProve, BuiltinProcessor>(),
        ProcessorCommand::Transfer { disable_link } => {
            tracing::debug!(disable_link = disable_link);
            run_consumer_with_proc::<Transfer, _>(TransferProcessor::new(*disable_link))
        }
        ProcessorCommand::WindowPoSt => run_consumer::<WindowPoSt, BuiltinProcessor>(),
        ProcessorCommand::WinningPoSt => run_consumer::<WinningPoSt, BuiltinProcessor>(),
    }
}
