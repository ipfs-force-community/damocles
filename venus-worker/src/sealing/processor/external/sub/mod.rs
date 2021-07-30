//! utilities for managing sub processes of processor

use std::collections::HashMap;
use std::env::{current_exe, vars};
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::thread;

use anyhow::{anyhow, Result};
use cgroups_rs::{cgroup_builder::CgroupBuilder, Cgroup};
use crossbeam_channel::{after, bounded, select, Receiver, Sender};
use serde::{Deserialize, Serialize};

use super::{
    super::{Input, Stage},
    config,
};

mod run;
pub use run::*;

mod process;
pub use process::*;

#[derive(Clone, Debug, Serialize, Deserialize)]
struct Response<T> {
    pub err_msg: Option<String>,
    pub result: Option<T>,
}

#[inline]
pub(super) fn ready_msg(name: &str) -> String {
    format!("{} processor ready", name)
}

pub(super) fn start_sub_process<I: Input>(
    cfg: &config::Ext,
    input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
) -> Result<SubProcess<I>> {
    let stage = I::STAGE;
    let name = format!("vcworker-sub-{}-{}", stage.name(), std::process::id());

    let (child, stdin, stdout) = start_child(stage, cfg)?;
    let cg = cfg.cgroup.as_ref().map(|c| start_cgroup(&name, c));
    if let Some(inner) = cg.as_ref() {
        inner.add_task((child.id() as u64).into())?;
    }

    let proc: SubProcess<_> = SubProcess::new(input_rx, name, child, stdin, stdout, cg);

    Ok(proc)
}

fn start_child(stage: Stage, cfg: &config::Ext) -> Result<(Child, ChildStdin, ChildStdout)> {
    let mut envs = HashMap::new();
    for (k, v) in vars() {
        envs.insert(k, v);
    }

    if let Some(mut set) = cfg.envs.as_ref().cloned() {
        for (k, v) in set.drain() {
            envs.insert(k, v);
        }
    }

    let bin = cfg
        .bin
        .as_ref()
        .cloned()
        .map(|s| Ok(PathBuf::from(s)))
        .unwrap_or(current_exe())?;

    let mut child = Command::new(bin)
        .args(&["processor", stage.name()])
        .envs(envs.drain())
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()?;

    let stdin = child.stdin.take().ok_or(anyhow!("child stdin not found"))?;

    let stdout = child
        .stdout
        .take()
        .ok_or(anyhow!("child stdout not found"))?;

    let (res_tx, res_rx) = bounded(1);
    let hdl = wait_for_process_ready(res_tx, stage, stdout);
    let wait = after(
        cfg.stable_wait
            .as_ref()
            .cloned()
            .unwrap_or(config::EXT_STABLE_WAIT),
    );

    select! {
        recv(res_rx) -> ready_res => {
            ready_res??
        },

        recv(wait) -> _ => {
            let _ = child.kill();
            return Err(anyhow!("timeout exceeded before child get ready"));
        }
    }

    let stdout = hdl
        .join()
        .map_err(|e| anyhow!("failed to recv stdout from spawned thread: {:?}", e))?;

    Ok((child, stdin, stdout))
}

fn wait_for_process_ready(
    res_tx: Sender<Result<()>>,
    stage: Stage,
    stdout: ChildStdout,
) -> thread::JoinHandle<ChildStdout> {
    std::thread::spawn(move || {
        let expected = ready_msg(stage.name());
        let mut line = String::with_capacity(expected.len() + 1);

        let mut buf = BufReader::new(stdout);
        let res = buf
            .read_line(&mut line)
            .map_err(|e| e.into())
            .and_then(|_| {
                if line.as_str().trim() == expected.as_str() {
                    Ok(())
                } else {
                    Err(anyhow!("unexpected first line: {}", line))
                }
            });

        let _ = res_tx.send(res);
        buf.into_inner()
    })
}

fn start_cgroup(name: &str, cfg: &config::Cgroup) -> Cgroup {
    let hier = cgroups_rs::hierarchies::auto();
    let mut builder = CgroupBuilder::new(name);
    if let Some(cpuset) = cfg.cpuset.as_ref().cloned() {
        builder = builder.cpu().cpus(cpuset).done();
    }

    builder.build(hier)
}
