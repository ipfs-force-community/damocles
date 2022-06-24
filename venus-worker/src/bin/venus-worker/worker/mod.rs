use std::fs;
use std::time::Duration;
use std::{io::Write, path::PathBuf};

use anyhow::{anyhow, Context, Result};
use clap::{value_t, App, AppSettings, Arg, ArgMatches, SubCommand};

use jsonrpc_core::ErrorCode;
use jsonrpc_core_client::RpcError;
use venus_worker::{
    block_on,
    client::{connect, WorkerClient},
    logging::info,
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
        .help("Enable external processor error response dump for debugging");

    let disable_dump_cmd = SubCommand::with_name("disable_dump")
        .arg(
            Arg::with_name("child_pid")
                .long("child_pid")
                .takes_value(true)
                .required(true)
                .help("Specify external processor pid"),
        )
        .help("Disable external processor error response dump");

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
                    "#{}: {:?}; sector_id={:?}, paused={}, paused_elapsed={:?}, state={}, last_err={:?}",
                    wi.index,
                    wi.location,
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

        ("enable_dump", Some(m)) => {
            let child_pid = value_t!(m, "child_pid", u32)?;
            let dump_dir: PathBuf = value_t!(m, "dump_dir", String)?.into();
            if !dump_dir.is_dir() {
                return Err(anyhow!("{} is not a directory", dump_dir.display()));
            }
            let dump_dir = fs::canonicalize(dump_dir)?;
            let env_name = vc_processors::core::ext::dump_error_resp_env(child_pid);

            get_client(subargs).and_then(|wcli| {
                block_on(wcli.worker_set_env(env_name, dump_dir.to_string_lossy().to_string())).map_err(|rpc_err| match rpc_err {
                    RpcError::JsonRpcError(e) if e.code == ErrorCode::InvalidParams => anyhow!(e.message),
                    _ => anyhow!("rpc error: {:?}", rpc_err),
                })
            })
        }

        ("disable_dump", Some(m)) => {
            let child_pid = value_t!(m, "child_pid", u32)?;
            let env_name = vc_processors::core::ext::dump_error_resp_env(child_pid);

            get_client(subargs).and_then(|wcli| {
                block_on(wcli.worker_remove_env(env_name)).map_err(|rpc_err| match rpc_err {
                    RpcError::JsonRpcError(e) if e.code == ErrorCode::InvalidParams => anyhow!(e.message),
                    _ => anyhow!("rpc error: {:?}", rpc_err),
                })
            })
        }

        (other, _) => Err(anyhow!("unexpected subcommand `{}` of worker", other)),
    }
}

fn get_client(m: &ArgMatches<'_>) -> Result<WorkerClient> {
    let cfg_path = value_t!(m, "config", String).context("get config path")?;
    let cfg = Config::load(&cfg_path)?;
    connect(&cfg)
}
