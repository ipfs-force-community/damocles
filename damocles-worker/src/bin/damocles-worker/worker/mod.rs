use std::fs;
use std::net::SocketAddr;
use std::{io::Write, path::PathBuf};

use anyhow::{anyhow, Context, Result};
use clap::{Parser, Subcommand};

use damocles_worker::{
    block_on,
    client::{connect, WorkerClient},
    logging::info,
    Config, DEFAULT_WORKER_SERVER_PORT, LOCAL_HOST,
};
use jsonrpc_core::ErrorCode;
use jsonrpc_core_client::RpcError;

#[derive(Parser)]
pub(crate) struct WorkerCommand {
    /// Daemon socket(s) to connect to
    #[arg(short = 'H', long, env = "DAMOCLES_WORKER_HOST")]
    host: Option<SocketAddr>,

    /// Path to the config file
    #[arg(short, long, value_name = "FILE", env = "DAMOCLES_WORKER_CONFIG")]
    config: Option<PathBuf>,

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
    let wcli = get_client(cmd.host.as_ref(), cmd.config.as_ref())?;
    match &cmd.subcommand {
        WorkerSubCommand::List => {
            use tabwriter::TabWriter;

            let infos = block_on(wcli.worker_list())
                .map_err(|e| anyhow!("rpc error: {:?}", e))?;
            let out = std::io::stdout();
            let hdl = out.lock();

            let mut tw = TabWriter::new(hdl);
            let _ = tw.write_fmt(format_args!(
                "#\tlocation\tPlan\tJobID\tJobState\tJobStage\tThreadState\tLastErr\n"
            ));
            for wi in infos {
                let _ = tw.write_fmt(format_args!(
                    "{}\t{}\t{}\t{}\t{}\t{}\t{}\t{}\n",
                    wi.index,
                    wi.location.display(),
                    wi.plan,
                    wi.job_id.as_deref().unwrap_or("-"),
                    wi.job_state.as_str(),
                    wi.job_stage.as_deref().unwrap_or("-"),
                    wi.thread_state,
                    wi.last_error.as_deref().unwrap_or("-").escape_debug()
                ));
            }
            let _ = tw.flush();
            Ok(())
        }
        WorkerSubCommand::Pause { index } => {
            let done = block_on(wcli.worker_pause(*index))
                .map_err(|e| anyhow!("rpc error: {:?}", e))?;
            info!(done, "#{} worker pause", index);
            Ok(())
        }
        WorkerSubCommand::Resume { index, state } => {
            let done = block_on(wcli.worker_resume(*index, state.clone()))
                .map_err(|e| anyhow!("rpc error: {:?}", e))?;
            info!(done, ?state, "#{} worker resume", index);
            Ok(())
        }
        WorkerSubCommand::EnableDump {
            child_pid,
            dump_dir,
        } => {
            if !dump_dir.is_dir() {
                return Err(anyhow!(
                    "'{}' is not a directory",
                    dump_dir.display()
                ));
            }
            let dump_dir = fs::canonicalize(dump_dir)?;
            let env_name =
                vc_processors::core::ext::dump_error_resp_env(*child_pid);

            block_on(wcli.worker_set_env(
                env_name,
                dump_dir.to_string_lossy().to_string(),
            ))
            .map_err(|rpc_err| match rpc_err {
                RpcError::JsonRpcError(e)
                    if e.code == ErrorCode::InvalidParams =>
                {
                    anyhow!(e.message)
                }
                _ => anyhow!("rpc error: {:?}", rpc_err),
            })
        }
        WorkerSubCommand::DisableDump { child_pid } => {
            let env_name =
                vc_processors::core::ext::dump_error_resp_env(*child_pid);

            block_on(wcli.worker_remove_env(env_name)).map_err(|rpc_err| {
                match rpc_err {
                    RpcError::JsonRpcError(e)
                        if e.code == ErrorCode::InvalidParams =>
                    {
                        anyhow!(e.message)
                    }
                    _ => anyhow!("rpc error: {:?}", rpc_err),
                }
            })
        }
    }
}

fn get_client(
    host: Option<&SocketAddr>,
    config: Option<&PathBuf>,
) -> Result<WorkerClient> {
    let host = match (host, config) {
        (None, None) => {
            let h = format!("{}:{}", LOCAL_HOST, DEFAULT_WORKER_SERVER_PORT);
            h.parse()
                .with_context(|| format!("parse connect address: {}", h))?
        }
        (Some(host), None) => *host,
        (None, Some(config)) | (Some(_), Some(config)) => {
            let cfg = Config::load(config)?;
            cfg.worker_server_connect_addr()?
        }
    };
    connect(host)
}
