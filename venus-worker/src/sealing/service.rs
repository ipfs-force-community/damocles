use crossbeam_channel::select;
use jsonrpc_core::{Error, IoHandler, Result};
use jsonrpc_ws_server::ServerBuilder;

use super::worker::Ctrl;

use crate::logging::{error, info};
use crate::rpc::worker::{Worker, WorkerInfo};
use crate::watchdog::{Ctx, Module};

const DEFAULT_PORT: u16 = 17890;
const DEFAULT_HOST: &str = "0.0.0.0";

struct ServiceImpl {
    ctrls: Vec<(usize, Ctrl)>,
}

impl ServiceImpl {
    fn get_ctrl(&self, index: usize) -> Result<&Ctrl> {
        self.ctrls
            .get(index)
            .map(|item| &item.1)
            .ok_or(Error::invalid_params(format!(
                "worker #{} not found",
                index
            )))
    }
}

impl Worker for ServiceImpl {
    fn list_workers(&self) -> Result<Vec<WorkerInfo>> {
        Ok(self
            .ctrls
            .iter()
            .map(|(idx, ctrl)| {
                let name: &str = ctrl.sealing_state.load().into();
                WorkerInfo {
                    location: ctrl.location.to_pathbuf(),
                    index: *idx,
                    paused: ctrl.paused.load(),
                    state: name.to_owned(),
                }
            })
            .collect())
    }

    fn pause_worker(&self, index: usize) -> Result<bool> {
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

    fn resume_worker(&self, index: usize, set_to: Option<String>) -> Result<bool> {
        let ctrl = self.get_ctrl(index)?;

        let state = set_to
            .map(|s| {
                s.parse()
                    .map_err(|e| Error::invalid_params(format!("{:?}", e)))
            })
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
}

pub struct Service {
    ctrls: Vec<(usize, Ctrl)>,
}

impl Service {
    pub fn new(ctrls: Vec<(usize, Ctrl)>) -> Self {
        Service { ctrls }
    }
}

impl Module for Service {
    fn id(&self) -> String {
        "worker-server".to_owned()
    }

    fn run(&mut self, ctx: Ctx) -> anyhow::Result<()> {
        let host = ctx
            .cfg
            .worker_server
            .as_ref()
            .and_then(|c| c.host.as_ref())
            .map(|s| s.as_str())
            .unwrap_or(DEFAULT_HOST);

        let port = ctx
            .cfg
            .worker_server
            .as_ref()
            .and_then(|c| c.port.as_ref())
            .cloned()
            .unwrap_or(DEFAULT_PORT);

        let srv_impl = ServiceImpl {
            ctrls: std::mem::take(&mut self.ctrls),
        };

        let mut io = IoHandler::new();
        io.extend_with(srv_impl.to_delegate());

        let addr = format!("{}:{}", host, port).parse()?;
        info!("listen on {:?}", addr);

        let server = ServerBuilder::new(io).start(&addr)?;

        server.wait()?;

        Ok(())
    }
}
