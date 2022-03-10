use anyhow::{anyhow, Result};
use clap::{App, AppSettings, ArgMatches, SubCommand};

use venus_worker::{
    run, run_c2, run_pc1, run_pc2, run_tree_d, SnapProveReplicaUpdateInput, SnapReplicaUpdateInput,
};

pub const SUB_CMD_NAME: &str = "processor";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let tree_d_cmd = SubCommand::with_name("tree_d");
    let pc1_cmd = SubCommand::with_name("pc1");
    let pc2_cmd = SubCommand::with_name("pc2");
    let c2_cmd = SubCommand::with_name("c2");
    let snap_replica_update_cmd = SubCommand::with_name("snap-ru");
    let snap_prove_replica_update_cmd = SubCommand::with_name("snap-pru");
    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(tree_d_cmd)
        .subcommand(pc1_cmd)
        .subcommand(pc2_cmd)
        .subcommand(c2_cmd)
        .subcommand(snap_replica_update_cmd)
        .subcommand(snap_prove_replica_update_cmd)
}

pub(crate) fn submatch<'a>(subargs: &ArgMatches<'a>) -> Result<()> {
    match subargs.subcommand() {
        ("pc1", _) => run_pc1(),

        ("pc2", _) => run_pc2(),

        ("c2", _) => run_c2(),

        ("tree_d", _) => run_tree_d(),

        ("snap-ru", _) => run::<SnapReplicaUpdateInput>(),

        ("snap-pru", _) => run::<SnapProveReplicaUpdateInput>(),

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of processor", other)),
    }
}
