use std::collections::HashMap;
use std::env::vars;
use std::io::{BufRead, BufReader, Write};
use std::os::raw::c_int;
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::sync::{
    atomic::{AtomicU64, Ordering},
    Arc, Mutex,
};
use std::thread;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{after, bounded, never, select, Sender};
use serde_json::{from_str, to_string};
use tracing::{debug, error, info, warn_span};

use super::{ready_msg, Request, Response};
use crate::core::{Processor, Task};

pub fn start_response_handler<T: Task>(stdout: ChildStdout, out_txes: Arc<Mutex<HashMap<u64, Sender<Response<T::Output>>>>>) -> Result<()> {
    let mut reader = BufReader::new(stdout);
    let mut line_buf = String::new();

    loop {
        line_buf.clear();

        let size = reader.read_line(&mut line_buf).context("read line from stdout")?;

        let resp: Response<T::Output> = match from_str(line_buf.as_str()) {
            Ok(r) => r,
            Err(e) => {
                error!("failed to unmarshal response tring: {:?}", e);
                continue;
            }
        };

        debug!(id = resp.id, size, "response received");
        // this should not be blocked
        if let Some(out_tx) = out_txes.lock().map_err(|_| anyhow!("out txes poisoned"))?.remove(&resp.id) {
            let _ = out_tx.send(resp);
        }
    }
}

pub struct Hooks<P, F> {
    pub prepare: Option<P>,
    pub finalize: Option<F>,
}

/// Builder for Producer
pub struct ProducerBuilder<HP, HF> {
    bin: String,
    args: Vec<String>,
    envs: HashMap<String, String>,

    inherit_envs: bool,
    stable_timeout: Option<Duration>,
    hooks: Hooks<HP, HF>,
}

impl<HP, HF> ProducerBuilder<HP, HF> {
    /// Construct a new builder with the given binary path & args.
    pub fn new(bin: String, args: Vec<String>) -> Self {
        ProducerBuilder {
            bin,
            args,
            envs: HashMap::new(),
            inherit_envs: true,
            stable_timeout: None,
            hooks: Hooks {
                prepare: None,
                finalize: None,
            },
        }
    }

    /// Set if we should inherit envs from the parent process, default is true.
    pub fn inherit_envs(&mut self, yes: bool) -> &mut Self {
        self.inherit_envs = yes;
        self
    }

    /// Set a pair of env name & value for the child.
    pub fn env(&mut self, name: String, value: String) -> &mut Self {
        self.envs.insert(name, value);
        self
    }

    /// Set the timeout before we wait for the child process to be stable, default is None, thus we
    /// would block util the child gives the ready message.
    pub fn stable_timeout(&mut self, timeout: Duration) -> &mut Self {
        self.stable_timeout.replace(timeout);
        self
    }

    /// Set if the child has a preferred numa node.
    #[cfg(feature = "numa")]
    pub fn numa_preferred(&mut self, node: c_int) -> &mut Self {
        self.env(crate::sys::numa::ENV_NUMA_PREFERRED.to_string(), node.to_string())
    }

    /// Set a prepare hook, which will be called before the Producer send the task to the child.
    pub fn hook_prepare(&mut self, f: HP) -> &mut Self {
        self.hooks.prepare.replace(f);
        self
    }

    /// Set a finalize hook, which will be called before the Processor::process returns.
    pub fn hook_finalize(&mut self, f: HF) -> &mut Self {
        self.hooks.finalize.replace(f);
        self
    }

    /// Build a Producer with the given options.
    pub fn build<T: Task>(mut self) -> Result<Producer<T, HP, HF>> {
        let mut envs = if self.inherit_envs {
            let mut envs = HashMap::from_iter(vars());
            envs.extend(self.envs.drain());
            envs
        } else {
            self.envs
        };

        let mut child = Command::new(self.bin)
            .args(self.args)
            .envs(envs.drain())
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .context("spawn command")?;

        let stdin = child.stdin.take().context("stdin lost")?;

        let stdout = child.stdout.take().context("stdout lost")?;

        // this will always be called until we turn defer.0 into false
        let mut defer = Defer(true, || {
            let _ = child.kill();
        });

        let (stable_tx, stable_rx) = bounded(1);
        let stable_hdl = wait_for_stable(stable_tx, T::STAGE, stdout);
        let wait = self.stable_timeout.take().map(after).unwrap_or_else(never);

        select! {
            recv(stable_rx) -> ready_res => {
                ready_res.context("stable chan broken")?.context("wait for stable")?
            },

            recv(wait) -> _ => {
                return Err(anyhow!("timeout exceeded before child get ready"));
            }
        }

        let stdout = stable_hdl.join().map_err(|_| anyhow!("wait for stable handle to be joined"))?;
        defer.0 = false;
        drop(defer);

        Ok(Producer {
            id: AtomicU64::new(0),
            child,
            stdin: Arc::new(Mutex::new(stdin)),
            stdout: Some(stdout),
            hooks: self.hooks,
            out_txes: Arc::new(Mutex::new(HashMap::new())),
        })
    }
}

fn wait_for_stable(res_tx: Sender<Result<()>>, stage: &'static str, stdout: ChildStdout) -> thread::JoinHandle<ChildStdout> {
    std::thread::spawn(move || {
        let expected = ready_msg(stage);
        let mut line = String::with_capacity(expected.len() + 1);

        let mut buf = BufReader::new(stdout);
        let res = buf.read_line(&mut line).map_err(|e| e.into()).and_then(|_| {
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

/// Producer sends tasks to the child, and waits for the responses.
/// It impl Processor.
pub struct Producer<T: Task, HP, HF> {
    id: AtomicU64,
    child: Child,
    stdin: Arc<Mutex<ChildStdin>>,
    stdout: Option<ChildStdout>,
    hooks: Hooks<HP, HF>,
    out_txes: Arc<Mutex<HashMap<u64, Sender<Response<T::Output>>>>>,
}

impl<T, HP, HF> Producer<T, HP, HF>
where
    T: Task,
{
    /// Returns the child's process id.
    pub fn child_pid(&self) -> u32 {
        self.child.id()
    }

    /// Starts response handler in background.
    /// This method should only be called once, and should not block.
    pub fn start_response_handler(&mut self) -> Result<()> {
        let stdout = self.stdout.take().context("stdout lost")?;

        let out_txes = self.out_txes.clone();
        thread::spawn(move || {
            let _span = warn_span!("response handler");
            if let Err(e) = start_response_handler::<T>(stdout, out_txes) {
                error!("unexpected: {:?}", e);
            }
        });

        Ok(())
    }

    fn send(&self, req: &Request<T>) -> Result<()> {
        let data = to_string(req).context("marshal request")?;
        let mut writer = self.stdin.lock().map_err(|_| anyhow!("stdin writer poisoned"))?;
        writeln!(writer, "{}", data).context("write request data")?;
        Ok(())
    }
}

impl<T: Task, HP, HF> Drop for Producer<T, HP, HF> {
    fn drop(&mut self) {
        let _span = warn_span!("drop", pid = self.child.id()).entered();
        info!("kill child");
        let _ = self.child.kill();
        let _ = self.child.wait();
        info!("cleaned up");
    }
}

impl<T, HP, HF> Processor for Producer<T, HP, HF>
where
    T: Task,
    HP: Fn(&Request<T>) -> Result<()> + Send + Sync,
    HF: Fn(&Request<T>) + Send + Sync,
{
    type Task = T;

    fn process(&self, task: T) -> Result<T::Output> {
        let req = Request {
            id: self.id.fetch_add(1, Ordering::SeqCst),
            task,
        };

        if let Some(p) = self.hooks.prepare.as_ref() {
            p(&req).context("prepare task")?;
        };

        let _defer = Defer(true, || {
            if let Some(f) = self.hooks.finalize.as_ref() {
                f(&req);
            }
        });

        let (output_tx, output_rx) = bounded(1);
        self.out_txes
            .lock()
            .map_err(|_| anyhow!("out txes poisoned"))?
            .insert(req.id, output_tx);

        if let Err(e) = self.send(&req) {
            self.out_txes.lock().map_err(|_| anyhow!("out txes poisoned"))?.remove(&req.id);
            return Err(e);
        }

        let mut output = output_rx.recv().map_err(|_| anyhow!("output channnel broken"))?;
        if let Some(err_msg) = output.err_msg.take() {
            return Err(anyhow!(err_msg));
        }

        output.output.take().context("output field lost")
    }
}

struct Defer<F: FnMut()>(bool, F);

impl<F: FnMut()> Drop for Defer<F> {
    fn drop(&mut self) {
        if self.0 {
            (self.1)();
        }
    }
}
