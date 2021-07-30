use std::io::{BufRead, BufReader, Write};
use std::process::{Child, ChildStdin, ChildStdout};

use anyhow::{anyhow, Result};
use crossbeam_channel::{select, Receiver, Sender};
use serde_json::{from_str, to_string};

use super::{super::Input, cgroup::CtrlGroup, Response};
use crate::{
    logging::{error, info},
    watchdog::{Ctx, Module},
};

/// an instance of a sub process
pub struct SubProcess<I: Input> {
    input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
    name: String,
    child: Child,
    stdin: ChildStdin,
    stdout: ChildStdout,
    _cgroup: Option<CtrlGroup>,
}

impl<I: Input> SubProcess<I> {
    pub(super) fn new(
        input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
        name: String,
        child: Child,
        stdin: ChildStdin,
        stdout: ChildStdout,
        cgroup: Option<CtrlGroup>,
    ) -> Self {
        SubProcess {
            input_rx,
            name,
            child,
            stdin,
            stdout,
            _cgroup: cgroup,
        }
    }

    fn handle_input(&mut self, input: I, line: &mut String) -> Result<I::Out> {
        let input_str = to_string(&input)?;
        writeln!(self.stdin, "{}", input_str)?;

        let mut buf = BufReader::new(&mut self.stdout);
        let size = buf.read_line(line)?;
        if size == 0 {
            return Err(anyhow!("stdout is closed unexpectedly"));
        }

        let mut response: Response<I::Out> = from_str(line.as_str())?;
        if let Some(out) = response.result.take() {
            return Ok(out);
        }

        if let Some(err_msg) = response.err_msg.take() {
            return Err(anyhow!("{}", err_msg));
        }

        Err(anyhow!("err without any message"))
    }
}

impl<I: Input> Module for SubProcess<I> {
    fn id(&self) -> String {
        format!("{}/{}", self.name, self.child.id())
    }

    fn run(&mut self, ctx: Ctx) -> Result<()> {
        let mut line = String::new();
        loop {
            let (input, out_tx) = select! {
                recv(ctx.done) -> _ => {
                    return Ok(())
                }

                recv(self.input_rx) -> input_res => {
                    input_res?
                }
            };

            line.clear();
            let out_res = self.handle_input(input, &mut line);

            select! {
                recv(ctx.done) -> _ => {
                    return Ok(())
                }

                send(out_tx, out_res) -> send_res => {
                    if let Err(_) = send_res {
                        error!("failed to send output through given chan");
                    }
                }
            }
        }
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
