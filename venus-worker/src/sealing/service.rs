use std::env;
use std::sync::Arc;

use crossbeam_channel::select;
use jsonrpc_core::{Error, IoHandler, Result};
use jsonrpc_http_server::ServerBuilder;
use tracing::{error, info};

use super::worker::Ctrl;

use crate::rpc::worker::{Worker, WorkerInfo};
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
            .map(|(idx, ctrl)| {
                let (state, sector_id, plan, last_error, paused_at) = ctrl
                    .load_state(|cst| {
                        (
                            cst.job.state,
                            cst.job.id.to_owned(),
                            cst.job.plan.clone(),
                            cst.job.last_error.to_owned(),
                            cst.paused_at.to_owned(),
                        )
                    })
                    .map_err(|e| {
                        let mut err = Error::internal_error();
                        err.message = e.to_string();
                        err
                    })?;

                Ok(WorkerInfo {
                    location: ctrl.location.to_pathbuf(),
                    plan,
                    sector_id,
                    index: *idx,
                    paused: paused_at.is_some(),
                    paused_elapsed: paused_at.map(|ins| ins.elapsed().as_secs()),
                    state: state.as_str().to_owned(),
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
