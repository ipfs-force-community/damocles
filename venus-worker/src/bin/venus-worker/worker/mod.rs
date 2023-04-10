use std::io::Write;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use clap::{value_t, App, AppSettings, Arg, ArgMatches, SubCommand};
use tracing::info;

use venus_worker::{
    block_on,
    client::{connect, WorkerClient},
    Config,
};

pub const SUB_CMD_NAME: &str = "worker";

pub fn subcommand<'a, 'b>() -> App<'a, 'b> {
    let list_cmd = SubCommand::with_name("list");
    let pause_cmd = SubCommand::with_name("pause").arg(
        Arg::with_name("index")
            .long("index")
            .short("i")
            .takes_value(true)
            .required(true)
            .help("index of the worker"),
    );

    let resume_cmd = SubCommand::with_name("resume")
        .arg(
            Arg::with_name("index")
                .long("index")
                .short("i")
                .takes_value(true)
                .required(true)
                .help("index of the worker"),
        )
        .arg(
            Arg::with_name("state")
                .long("state")
                .short("s")
                .takes_value(true)
                .required(false)
                .help("next state"),
        );

    let enable_dump_cmd = SubCommand::with_name("enable_dump")
        .args(&[
            Arg::with_name("child_pid")
                .long("child_pid")
                .takes_value(true)
                .required(true)
                .help("Specify external processor pid"),
            Arg::with_name("dump_dir")
                .long("dump_dir")
                .takes_value(true)
                .required(true)
                .help("Specify the dump directory"),
        ])
        .help("Enable external processor error format response dump for debugging");

    let disable_dump_cmd = SubCommand::with_name("disable_dump")
        .arg(
            Arg::with_name("child_pid")
                .long("child_pid")
                .takes_value(true)
                .required(true)
                .help("Specify external processor pid"),
        )
        .help("Disable external processor error format response dump");

    SubCommand::with_name(SUB_CMD_NAME)
        .setting(AppSettings::ArgRequiredElseHelp)
        .arg(
            Arg::with_name("config")
                .long("config")
                .short("c")
                .takes_value(true)
                .required(true)
                .help("path to the config file"),
        )
        .subcommand(list_cmd)
        .subcommand(pause_cmd)
        .subcommand(resume_cmd)
        .subcommand(enable_dump_cmd)
        .subcommand(disable_dump_cmd)
}

pub fn submatch(subargs: &ArgMatches<'_>) -> Result<()> {
    match subargs.subcommand() {
        ("list", _) => get_client(subargs).and_then(|wcli| {
            let infos = block_on(wcli.worker_list()).map_err(|e| anyhow!("rpc error: {:?}", e))?;
            let out = std::io::stdout();
            let mut hdl = out.lock();
            for wi in infos {
                let _ = writeln!(
                    &mut hdl,
                    "#{}: {:?}; plan={}, sector_id={:?}, paused={}, paused_elapsed={:?}, state={}, last_err={:?}",
                    wi.index,
                    wi.location,
                    wi.plan,
                    wi.sector_id,
                    wi.paused,
                    wi.paused_elapsed.map(Duration::from_secs),
                    wi.state.as_str(),
                    wi.last_error
                );
            }

            Ok(())
        }),

        ("pause", Some(m)) => {
            let index = value_t!(m, "index", usize)?;
            get_client(subargs).and_then(|wcli| {
                let done = block_on(wcli.worker_pause(index)).map_err(|e| anyhow!("rpc error: {:?}", e))?;

                info!(done, "#{} worker pause", index);
                Ok(())
            })
        }

        ("resume", Some(m)) => {
            let index = value_t!(m, "index", usize)?;
            let state = m.value_of("state").map(|s| s.to_owned());
            get_client(subargs).and_then(|wcli| {
                let done = block_on(wcli.worker_resume(index, state.clone())).map_err(|e| anyhow!("rpc error: {:?}", e))?;

                info!(done, ?state, "#{} worker resume", index);
                Ok(())
            })
        }
        (other, _) => Err(anyhow!("unexpected subcommand `{}` of worker", other)),
    }
}

fn get_client(m: &ArgMatches<'_>) -> Result<WorkerClient> {
    let cfg_path = value_t!(m, "config", String).context("get config path")?;
    let cfg = Config::load(cfg_path)?;
    connect(&cfg)
}
