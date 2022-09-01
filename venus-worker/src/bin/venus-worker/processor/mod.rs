use anyhow::{anyhow, Result};
use clap::{App, AppSettings, ArgMatches, SubCommand};
use vc_processors::{
    builtin::{
        processors::BuiltinProcessor,
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
    let pc1_cmd = SubCommand::with_name(STAGE_NAME_PC1);
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
        (STAGE_NAME_ADD_PIECES, _) => run_consumer::<AddPieces, BuiltinProcessor>(),

        (STAGE_NAME_PC1, _) => run_consumer::<PC1, BuiltinProcessor>(),

        (STAGE_NAME_PC2, _) => run_consumer::<PC2, BuiltinProcessor>(),

        (STAGE_NAME_C2, _) => run_consumer::<C2, BuiltinProcessor>(),

        (STAGE_NAME_TREED, _) => run_consumer::<TreeD, BuiltinProcessor>(),

        (STAGE_NAME_SNAP_ENCODE, _) => run_consumer::<SnapEncode, BuiltinProcessor>(),

        (STAGE_NAME_SNAP_PROVE, _) => run_consumer::<SnapProve, BuiltinProcessor>(),

        (STAGE_NAME_TRANSFER, _) => run_consumer::<Transfer, BuiltinProcessor>(),

        (STAGE_NAME_WINDOW_POST, _) => run_consumer::<WindowPoSt, BuiltinProcessor>(),

        (STAGE_NAME_WINNING_POST, _) => run_consumer::<WinningPoSt, BuiltinProcessor>(),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of processor", other)),
    }
}
