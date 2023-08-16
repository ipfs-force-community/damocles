use std::env;
use std::path::PathBuf;
use std::sync::Arc;

use crossbeam_channel::select;
use jsonrpc_core::{Error, IoHandler, Result};
use jsonrpc_http_server::ServerBuilder;

use super::sealing_thread::{self, Ctrl};

use crate::logging::{error, info};
use crate::rpc::worker::{SealingThreadState, Worker, WorkerInfo};
use crate::watchdog::{Ctx, Module};

struct ServiceImpl {
    ctrls: Arc<Vec<(usize, Ctrl)>>,
}

impl ServiceImpl {
    fn get_ctrl(&self, index: usize) -> Result<&Ctrl> {
        self.ctrls
            .get(index)
            .map(|item| &item.1)
            .ok_or_else(|| Error::invalid_params(format!("worker #{} not found", index)))
    }
}

impl Worker for ServiceImpl {
    fn worker_list(&self) -> Result<Vec<WorkerInfo>> {
        self.ctrls
            .iter()
            .map(|(idx, ctrl)| -> std::result::Result<WorkerInfo, Error> {
                let (job_state, job_stage, job_id, plan, last_error, sealing_thread_state) = ctrl
                    .load_state(|cst| {
                        (
                            cst.job.state.clone(),
                            cst.job.stage.clone(),
                            cst.job.id.to_owned(),
                            cst.job.plan.clone(),
                            cst.job.last_error.to_owned(),
                            cst.state.clone(),
                        )
                    })
                    .map_err(|e| {
                        let mut err = Error::internal_error();
                        err.message = e.to_string();
                        err
                    })?;

                Ok(WorkerInfo {
                    location: ctrl
                        .location
                        .as_ref()
                        .map(|loc| loc.to_pathbuf())
                        .unwrap_or_else(|| PathBuf::from("-")),
                    plan,
                    job_id,
                    index: *idx,
                    thread_state: match sealing_thread_state {
                        sealing_thread::SealingThreadState::Idle => SealingThreadState::Idle,
                        sealing_thread::SealingThreadState::PausedAt(at) => SealingThreadState::Paused {
                            elapsed: at.elapsed().as_secs(),
                        },
                        sealing_thread::SealingThreadState::Running { at, proc } => SealingThreadState::Running {
                            elapsed: at.elapsed().as_secs(),
                            proc,
                        },
                        sealing_thread::SealingThreadState::WaitAt(at) => SealingThreadState::Waiting {
                            elapsed: at.elapsed().as_secs(),
                        },
                    },
                    job_state: job_state.unwrap_or(String::new()),
                    job_stage,
                    last_error,
                })
            })
            .collect::<Result<_>>()
    }

    fn worker_pause(&self, index: usize) -> Result<bool> {
        let ctrl = self.get_ctrl(index)?;

        select! {
            send(ctrl.pause_tx, ()) -> pause_res => {
                pause_res.map_err(|e| {
                    error!("pause chan broke: {:?}", e);
                    Error::internal_error()
                })?;

                Ok(true)
            }

            default => {
                Ok(false)
            }
        }
    }

    fn worker_resume(&self, index: usize, set_to: Option<String>) -> Result<bool> {
        let ctrl = self.get_ctrl(index)?;

        let state = set_to
            .map(|s| s.parse().map_err(|e| Error::invalid_params(format!("{:?}", e))))
            .transpose()?;

        select! {
            send(ctrl.resume_tx, state) -> resume_res => {
                resume_res.map_err(|e| {
                    error!("resume chan broke: {:?}", e);
                    Error::internal_error()
                })?;
                Ok(true)
            }

            default => {
                Ok(false)
            }
        }
    }

    fn worker_set_env(&self, name: String, value: String) -> Result<()> {
        // Avoid panic of set_var function
        // See: https://doc.rust-lang.org/stable/std/env/fn.set_var.html#panics
        if name.is_empty()
            || name.contains(['=', '\0'])
            || value.contains('\0')
            || std::panic::catch_unwind(|| env::set_var(name, value)).is_err()
        {
            return Err(Error::invalid_params("invalid environment variable name or value"));
        }
        Ok(())
    }

    fn worker_remove_env(&self, name: String) -> Result<()> {
        // Avoid panic of remove_var function
        // See: https://doc.rust-lang.org/std/env/fn.remove_var.html#panics
        if name.is_empty() || name.contains(['=', '\0']) || std::panic::catch_unwind(|| env::remove_var(name)).is_err() {
            return Err(Error::invalid_params("invalid environment variable name"));
        }
        Ok(())
    }

    fn worker_version(&self) -> Result<String> {
        Ok((*crate::version::VERSION).clone())
    }
}

pub struct Service {
    ctrls: Arc<Vec<(usize, Ctrl)>>,
}

impl Service {
    pub fn new(ctrls: Arc<Vec<(usize, Ctrl)>>) -> Self {
        Service { ctrls }
    }
}

impl Module for Service {
    fn should_wait(&self) -> bool {
        false
    }

    fn id(&self) -> String {
        "worker-server".to_owned()
    }

    fn run(&mut self, ctx: Ctx) -> anyhow::Result<()> {
        let addr = ctx.cfg.worker_server_listen_addr()?;

        let srv_impl = ServiceImpl { ctrls: self.ctrls.clone() };

        let mut io = IoHandler::new();
        io.extend_with(srv_impl.to_delegate());

        info!("listen on {:?}", addr);

        let server = ServerBuilder::new(io).threads(8).start_http(&addr)?;

        server.wait();

        Ok(())
    }
}
