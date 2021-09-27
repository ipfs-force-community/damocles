//! utilities for managing sub processes of processor

use std::collections::HashMap;
use std::env::{current_exe, vars};
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::thread;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{after, bounded, select, Receiver, Sender};
use serde::{Deserialize, Serialize};

use super::{
    super::{Input, Stage},
    config,
};
use crate::logging::info;

mod run;
pub use run::*;

mod process;
pub use process::*;

mod cgroup;

#[derive(Clone, Debug, Serialize, Deserialize)]
struct Response<T> {
    pub err_msg: Option<String>,
    pub result: Option<T>,
}

#[inline]
pub(super) fn ready_msg(name: &str) -> String {
    format!("{} processor ready", name)
}

pub(super) fn start_sub_processes<I: Input>(
    cfg: &config::Ext,
    input_rx: Receiver<(I, Sender<Result<I::Out>>)>,
) -> Result<Vec<SubProcess<I>>> {
    let sub_cfgs = match &cfg.subs {
        Some(s) if s.len() > 0 => s,

        _ => {
            return Err(anyhow!("no subs section found"));
        }
    };

    let mut processes = Vec::with_capacity(sub_cfgs.len());
    let stage = I::STAGE;

    for (i, sub_cfg) in sub_cfgs.iter().enumerate() {
        let name = format!("sub-{}-{}-{}", stage.name(), std::process::id(), i);
        let (child, stdin, stdout) = start_child(stage, sub_cfg)?;

        let mut cg = sub_cfg
            .cgroup
            .as_ref()
            .map(|c| cgroup::CtrlGroup::new(&name, c))
            .transpose()?;

        if let Some(inner) = cg.as_mut() {
            let pid = child.id() as u64;
            info!(child = pid, group = name.as_str(), "add into cgroup");
            inner
                .add_task(pid.into())
                .context("add task id into cgroup")?;
        }

        let proc: SubProcess<_> = SubProcess::new(input_rx.clone(), name, child, stdin, stdout, cg);
        processes.push(proc);
    }

    Ok(processes)
}

fn start_child(stage: Stage, cfg: &config::ExtSub) -> Result<(Child, ChildStdin, ChildStdout)> {
    let mut envs = HashMap::new();
    for (k, v) in vars() {
        envs.insert(k, v);
    }

    if let Some(mut set) = cfg.envs.as_ref().cloned() {
        for (k, v) in set.drain() {
            envs.insert(k, v);
        }
    }

    #[cfg(feature = "numa")]
    if let Some(preferred) = cfg.numa_preferred {
        envs.insert(
            crate::sys::numa::ENV_NUMA_PREFERRED.to_owned(),
            preferred.to_string(),
        );
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
