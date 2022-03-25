use std::collections::HashMap;
use std::io::{BufRead, BufReader, Write};
use std::process::{Child, ChildStdin, ChildStdout};

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, select, Receiver, Sender};
use serde_json::{from_str, to_string};

use super::{super::Input, cgroup::CtrlGroup, Request, Response};
use crate::{
    logging::{debug, error, info, warn_span},
    watchdog::{Ctx, Module},
};

/// an instance of a sub process
pub struct SubProcess<I: Input> {
    input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
    limit_tx: Sender<()>,
    limit_rx: Receiver<()>,
    response_rx: Receiver<Response<I::Out>>,
    read_done: Receiver<()>,
    name: String,
    child: Child,
    stdin: ChildStdin,
    read_ctx: Option<(Sender<()>, Sender<Response<I::Out>>, ChildStdout)>,
    counter: u64,
    out_txs: HashMap<u64, Sender<Result<I::Out>>>,
    _cgroup: Option<CtrlGroup>,
}

impl<I: Input> SubProcess<I> {
    pub(super) fn new(
        input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
        limit_tx: Sender<()>,
        limit_rx: Receiver<()>,
        name: String,
        child: Child,
        stdin: ChildStdin,
        stdout: ChildStdout,
        cgroup: Option<CtrlGroup>,
    ) -> Self {
        let (response_tx, response_rx) = bounded(1);
        let (read_tx, read_done) = bounded(0);
        SubProcess {
            input_rx,
            limit_tx,
            limit_rx,
            response_rx,
            read_done,
            name,
            child,
            stdin,
            read_ctx: Some((read_tx, response_tx, stdout)),
            counter: 0,
            out_txs: HashMap::new(),
            _cgroup: cgroup,
        }
    }

    fn handle_input(&mut self, id: u64, input: I) -> Result<()> {
        let req = Request { id, data: input };
        let req_str = to_string(&req)?;
        writeln!(self.stdin, "{}", req_str)?;
        Ok(())
    }
}

fn start_readline<I: Input>(
    mod_id: String,
    done: Receiver<()>,
    _read_tx: Sender<()>,
    response_tx: Sender<Response<I::Out>>,
    stdout: ChildStdout,
) -> Result<()> {
    let _enter = warn_span!("readline thread", %mod_id).entered();
    let mut buf = BufReader::new(stdout);
    let mut line = String::new();
    loop {
        line.clear();
        let size = buf.read_line(&mut line).context("read line from stdout")?;
        if size == 0 {
            return Ok(());
        }

        let resp: Response<I::Out> = match from_str(line.as_str()) {
            Ok(r) => r,
            Err(e) => {
                error!("failed to unmarshal response tring: {:?}", e);
                continue;
            }
        };

        debug!(id = resp.id, size, "response received");
        select! {
            recv(done) -> _ => {
                return Ok(())
            }

            send(response_tx, resp) -> send_res => {
                if let Err(e) = send_res {
                    return Err(anyhow!("response tx broken: {:?}", e));
                }
            }
        }
    }
}

impl<I: Input> Module for SubProcess<I> {
    fn id(&self) -> String {
        format!("{}/{}", self.name, self.child.id())
    }

    fn run(&mut self, ctx: Ctx) -> Result<()> {
        let (read_tx, response_tx, stdout) =
            self.read_ctx.take().context("read context required")?;
        let mod_id = self.id();
        let done = ctx.done.clone();
        let _ = std::thread::spawn(|| {
            if let Err(e) = start_readline::<I>(mod_id, done, read_tx, response_tx, stdout) {
                error!("read line thread exit: {:?}", e);
            }
        });

        let mut pending_input = None;
        let mut pending_response = None;
        loop {
            match pending_input.take() {
                Some((id, input)) => {
                    debug!("check for limitation");
                    select! {
                        recv(ctx.done) -> _ => {
                            return Ok(());
                        }

                        recv(self.read_done) -> _ => {
                            return Err(anyhow!("readline thread ended unexpectedly"));
                        }

                        send(self.limit_tx, ()) -> send_res => {
                            send_res.context("limit tx broken")?;
                            if let Err(e) = self.handle_input(id, input) {
                                pending_response.replace(Response {
                                    id,
                                    err_msg: Some(format!("handle input: {:?}", e)),
                                    result: None,
                                });
                            } else {
                                debug!(id, "handled");
                            }
                        }

                        recv(self.response_rx) -> recv_res => {
                            let resp = recv_res.context("response rx broken")?;
                            // refill the pending input
                            pending_input.replace((id, input));
                            pending_response.replace(resp);
                        }
                    };
                }

                None => {
                    debug!("waiting for next input or output");
                    select! {
                        recv(ctx.done) -> _ => {
                            return Ok(());
                        }

                        recv(self.read_done) -> _ => {
                            return Err(anyhow!("readline thread ended unexpectedly"));
                        }

                        recv(self.input_rx) -> input_res => {
                            let (input, out_tx) = input_res.context("input rx broken")?;
                            let id = self.counter;
                            self.counter += 1;
                            debug!(id, "requested");
                            self.out_txs.insert(id, out_tx);
                            pending_input.replace((id, input));
                        }

                        recv(self.response_rx) -> recv_res => {
                            let resp = recv_res.context("response rx broken")?;
                            pending_response.replace(resp);
                        }
                    };
                }
            }

            if let Some(mut resp) = pending_response.take() {
                // TODO: check this?
                let _ = self.limit_rx.try_recv().map(drop);

                match self.out_txs.remove(&resp.id) {
                    Some(out_tx) => {
                        let out_res = if let Some(out) = resp.result.take() {
                            Ok(out)
                        } else if let Some(err_msg) = resp.err_msg.take() {
                            Err(anyhow!("{}", err_msg))
                        } else {
                            Err(anyhow!("err without any message"))
                        };

                        select! {
                            recv(ctx.done) -> _ => {
                                return Ok(())
                            }

                            send(out_tx, out_res) -> send_res => {
                                if let Err(_) = send_res {
                                    error!(id = resp.id, "failed to send output through given chan");
                                } else {
                                    debug!(id = resp.id, "responsed");
                                }
                            }
                        };
                    }

                    None => {
                        error!(id = resp.id, "output tx not found");
                    }
                }
            }
        }
    }

    fn should_wait(&self) -> bool {
        true
    }
}

impl<I: Input> Drop for SubProcess<I> {
    fn drop(&mut self) {
        info!("kill child");
        let _ = self.child.kill();
        let _ = self.child.wait();
        info!("cleaned up");
    }
}
