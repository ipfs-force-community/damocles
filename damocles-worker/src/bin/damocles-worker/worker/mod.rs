use std::fs;
use std::path::Path;
use std::time::Duration;
use std::{io::Write, path::PathBuf};

use anyhow::{anyhow, Result};
use clap::{Parser, Subcommand};

use damocles_worker::{
    block_on,
    client::{connect, WorkerClient},
    logging::info,
    Config,
};
use jsonrpc_core::ErrorCode;
use jsonrpc_core_client::RpcError;

#[derive(Parser)]
pub(crate) struct WorkerCommand {
    /// Path to the config file
    #[arg(short = 'c', long, value_name = "FILE")]
    config: PathBuf,

    #[command(subcommand)]
    subcommand: WorkerSubCommand,
}

/// Group of commands for control worker
#[derive(Subcommand)]
enum WorkerSubCommand {
    /// List all sealing_thread
    List,
    Pause {
        /// index of the worker
        #[arg(short, long)]
        index: usize,
    },
    Resume {
        /// index of the worker
        #[arg(short, long)]
        index: usize,
        /// next state
        #[arg(short, long)]
        state: Option<String>,
    },
    /// Enable external processor dump error response for debugging
    #[command(alias = "enable_dump")]
    EnableDump {
        /// Specify external processor pid
        #[arg(long, alias = "child_pid")]
        child_pid: u32,
        /// Specify the dump directory
        #[arg(long, alias = "dump_dir")]
        dump_dir: PathBuf,
    },
    /// Disable external processor dump error response
    #[command(alias = "disable_dump")]
    DisableDump {
        /// Specify external processor pid
        #[arg(long, alias = "child_pid")]
        child_pid: u32,
    },
}

pub(crate) fn run(cmd: &WorkerCommand) -> Result<()> {
    let wcli = get_client(&cmd.config)?;
    match &cmd.subcommand {
        WorkerSubCommand::List => {
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
        }
        WorkerSubCommand::Pause { index } => {
            let done = block_on(wcli.worker_pause(*index)).map_err(|e| anyhow!("rpc error: {:?}", e))?;
            info!(done, "#{} worker pause", index);
            Ok(())
        }
        WorkerSubCommand::Resume { index, state } => {
            let done = block_on(wcli.worker_resume(*index, state.clone())).map_err(|e| anyhow!("rpc error: {:?}", e))?;
            info!(done, ?state, "#{} worker resume", index);
            Ok(())
        }
        WorkerSubCommand::EnableDump { child_pid, dump_dir } => {
            if !dump_dir.is_dir() {
                return Err(anyhow!("'{}' is not a directory", dump_dir.display()));
            }
            let dump_dir = fs::canonicalize(dump_dir)?;
            let env_name = vc_processors::core::ext::dump_error_resp_env(*child_pid);

            block_on(wcli.worker_set_env(env_name, dump_dir.to_string_lossy().to_string())).map_err(|rpc_err| match rpc_err {
                RpcError::JsonRpcError(e) if e.code == ErrorCode::InvalidParams => anyhow!(e.message),
                _ => anyhow!("rpc error: {:?}", rpc_err),
            })
        }
        WorkerSubCommand::DisableDump { child_pid } => {
            let env_name = vc_processors::core::ext::dump_error_resp_env(*child_pid);

            block_on(wcli.worker_remove_env(env_name)).map_err(|rpc_err| match rpc_err {
                RpcError::JsonRpcError(e) if e.code == ErrorCode::InvalidParams => anyhow!(e.message),
                _ => anyhow!("rpc error: {:?}", rpc_err),
            })
        }
    }
}

fn get_client(config_path: impl AsRef<Path>) -> Result<WorkerClient> {
    let cfg = Config::load(config_path)?;
    connect(&cfg)
}
