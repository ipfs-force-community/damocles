use anyhow::{anyhow, Context, Result};
use clap::{App, AppSettings, Arg, ArgMatches, SubCommand};
use vc_processors::{
    builtin::{
        executor::BuiltinTaskExecutor,
        tasks::{
            AddPieces, SnapEncode, SnapProve, Transfer, TreeD, WindowPoSt, WinningPoSt, C2, PC1, PC2, STAGE_NAME_ADD_PIECES, STAGE_NAME_C2,
            STAGE_NAME_PC1, STAGE_NAME_PC2, STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TRANSFER, STAGE_NAME_TREED,
            STAGE_NAME_WINDOW_POST, STAGE_NAME_WINNING_POST,
        },
    },
    core::ext::run_consumer,
};

pub const SUB_CMD_NAME: &str = "processor";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let add_pieces_cmd = SubCommand::with_name(STAGE_NAME_ADD_PIECES);
    let tree_d_cmd = SubCommand::with_name(STAGE_NAME_TREED);
    let pc1_cmd = SubCommand::with_name(STAGE_NAME_PC1)
        .arg(
            Arg::with_name("hugepage_files_path")
                .long("hugepage_files_path")
                .env("HUGEPAGE_FILES_PATH")
                .takes_value(true)
                .required(false)
                .long_help(
                    "Specify the path to the hugepage memory file and scan the hugepage memory files 
using the default pattern (/specified_hugepage_file_path/numa_$NUMA_NODE_INDEX).
It will match:
/specified_hugepage_file_path/numa_0/any_files
/specified_hugepage_file_path/numa_1/any_files
/specified_hugepage_file_path/numa_2/any_files
...

Make sure that the memory files stored in the folder are created in the numa node corresponding to $NUMA_NODE_INDEX.
This argument will be ignored if `hugepage_files_path_pattern` is specified.",
                ),
        )
        .arg(
            Arg::with_name("hugepage_files_path_pattern")
                .long("hugepage_files_path_pattern")
                .env("HUGEPAGE_FILES_PATH_PATTERN")
                .required(false)
                .takes_value(true)
                .long_help(
                    "Specify the hugepage memory file path pattern where $NUMA_NODE_INDEX represents 
the numa node index placeholder, which extracts the number in the folder name as the numa node index
Make sure that the memory files stored in the folder are created in the numa node corresponding to $NUMA_NODE_INDEX.
If both the argument `hugepage_files_path` and the argument `hugepage_files_path_pattern` are specified,
the argument `hugepage_files_path` will be ignored.",
                ),
        );
    let pc2_cmd = SubCommand::with_name(STAGE_NAME_PC2);
    let c2_cmd = SubCommand::with_name(STAGE_NAME_C2);
    let snap_encode_cmd = SubCommand::with_name(STAGE_NAME_SNAP_ENCODE);
    let snap_prove_cmd = SubCommand::with_name(STAGE_NAME_SNAP_PROVE);
    let transfer_cmd = SubCommand::with_name(STAGE_NAME_TRANSFER);
    let window_post_cmd = SubCommand::with_name(STAGE_NAME_WINDOW_POST);
    let winning_post_cmd = SubCommand::with_name(STAGE_NAME_WINNING_POST);

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(add_pieces_cmd)
        .subcommand(tree_d_cmd)
        .subcommand(pc1_cmd)
        .subcommand(pc2_cmd)
        .subcommand(c2_cmd)
        .subcommand(snap_encode_cmd)
        .subcommand(snap_prove_cmd)
        .subcommand(transfer_cmd)
        .subcommand(window_post_cmd)
        .subcommand(winning_post_cmd)
}

pub(crate) fn submatch(subargs: &ArgMatches<'_>) -> Result<()> {
    match subargs.subcommand() {
        (STAGE_NAME_ADD_PIECES, _) => run_consumer::<AddPieces, BuiltinTaskExecutor>(),

        (STAGE_NAME_PC1, Some(m)) => {
            use storage_proofs_porep::stacked::init_numa_mem_pool;
            use venus_worker::seal_util::{scan_memory_files, MemoryFileDirPattern};

            // Argument `hugepage_files_path_pattern` take precedence over argument `hugepage_files_path`
            match (
                m.value_of("hugepage_files_path").map(MemoryFileDirPattern::new_default),
                m.value_of("hugepage_files_path_pattern").map(MemoryFileDirPattern::without_prefix),
            ) {
                (Some(_), Some(p)) | (Some(p), None) | (None, Some(p)) => {
                    init_numa_mem_pool(scan_memory_files(&p).context("scan_memory_files")?);
                }
                (None, None) => {}
            }

            run_consumer::<PC1, BuiltinTaskExecutor>()
        }

        (STAGE_NAME_PC2, _) => run_consumer::<PC2, BuiltinTaskExecutor>(),

        (STAGE_NAME_C2, _) => run_consumer::<C2, BuiltinTaskExecutor>(),

        (STAGE_NAME_TREED, _) => run_consumer::<TreeD, BuiltinTaskExecutor>(),

        (STAGE_NAME_SNAP_ENCODE, _) => run_consumer::<SnapEncode, BuiltinTaskExecutor>(),

        (STAGE_NAME_SNAP_PROVE, _) => run_consumer::<SnapProve, BuiltinTaskExecutor>(),

        (STAGE_NAME_TRANSFER, _) => run_consumer::<Transfer, BuiltinTaskExecutor>(),

        (STAGE_NAME_WINDOW_POST, _) => run_consumer::<WindowPoSt, BuiltinTaskExecutor>(),

        (STAGE_NAME_WINNING_POST, _) => run_consumer::<WinningPoSt, BuiltinTaskExecutor>(),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of processor", other)),
    }
}
