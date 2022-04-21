use anyhow::{anyhow, Result};
use clap::{App, AppSettings, ArgMatches, SubCommand};

use venus_worker::{
    run, run_c2, run_pc1, run_pc2, run_tree_d, SnapEncodeInput, SnapProveInput, STAGE_NAME_C2, STAGE_NAME_PC1, STAGE_NAME_PC2,
    STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE, STAGE_NAME_TREED,
};

pub const SUB_CMD_NAME: &str = "processor";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let tree_d_cmd = SubCommand::with_name(STAGE_NAME_TREED);
    let pc1_cmd = SubCommand::with_name(STAGE_NAME_PC1);
    let pc2_cmd = SubCommand::with_name(STAGE_NAME_PC2);
    let c2_cmd = SubCommand::with_name(STAGE_NAME_C2);
    let snap_encode_cmd = SubCommand::with_name(STAGE_NAME_SNAP_ENCODE);
    let snap_prove_cmd = SubCommand::with_name(STAGE_NAME_SNAP_PROVE);
    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(tree_d_cmd)
        .subcommand(pc1_cmd)
        .subcommand(pc2_cmd)
        .subcommand(c2_cmd)
        .subcommand(snap_encode_cmd)
        .subcommand(snap_prove_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        (STAGE_NAME_PC1, _) => run_pc1(),

        (STAGE_NAME_PC2, _) => run_pc2(),

        (STAGE_NAME_C2, _) => run_c2(),

        (STAGE_NAME_TREED, _) => run_tree_d(),

        (STAGE_NAME_SNAP_ENCODE, _) => run::<SnapEncodeInput>(),

        (STAGE_NAME_SNAP_PROVE, _) => run::<SnapProveInput>(),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of processor", other)),
    }
}
