use std::collections::HashMap;
use std::env::{self, vars};
use std::fs;
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
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
use tracing::{debug, error, info, warn, warn_span};
use uuid::Uuid;

use super::{ready_msg, Request, Response};
use crate::core::{Processor, Task};

/// Dump error resp env key
pub fn dump_error_resp_env(pid: u32) -> String {
    format!("DUMP_ERR_RESP_{}", pid)
}

pub fn start_response_handler<T: Task>(
    pid: u32,
    stdout: ChildStdout,
    out_txes: Arc<Mutex<HashMap<u64, Sender<Response<T::Output>>>>>,
) -> Result<()> {
    let mut reader = BufReader::new(stdout);
    let mut line_buf = String::new();

    loop {
        line_buf.clear();

        let size = reader.read_line(&mut line_buf).context("read line from stdout")?;
        if size == 0 {
            warn!("child exited");
            return Ok(());
        }

        let resp: Response<T::Output> = match from_str(line_buf.as_str()) {
            Ok(r) => r,
            Err(e) => {
                dump(&DumpType::from_env(pid), e, line_buf.as_str());
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

/// Dump type of the error response of the processor
#[derive(Debug, Clone)]
enum DumpType {
    /// Dump error response fragments to the log
    ToLog,
    /// Dump error response fragments to the file
    ToFile(PathBuf),
}

impl DumpType {
    fn from_env(pid: u32) -> Self {
        match env::var(dump_error_resp_env(pid)) {
            Ok(path) => Self::ToFile(path.into()),
            Err(_) => Self::ToLog,
        }
    }
}

fn dump(dt: &DumpType, serde_err: serde_json::Error, data: &str) {
    #[inline]
    fn truncate(data: &str) -> String {
        const TRUNCATE_SIZE: usize = 100;

        if data.len() <= TRUNCATE_SIZE {
            return data.to_string();
        }

        let trunc_len = (0..TRUNCATE_SIZE + 1).rposition(|index| data.is_char_boundary(index)).unwrap_or(0);
        format!("{}...", &data[..trunc_len])
    }

    match dt {
        DumpType::ToLog => {
            error!(serde_json_err=%serde_err, "failed to unmarshal response string: '{}'", truncate(data));
        }
        DumpType::ToFile(dir) => match dump_to_file(dir, data.as_bytes()) {
            Ok(dump_file_path) => {
                error!(serde_json_err=%serde_err,
                    "failed to unmarshal response string. dump file '{}' generated",
                    dump_file_path.display()
                )
            }
            Err(e) => {
                error!(serde_json_err=%serde_err, "failed to unmarshal response string; failed to generate dump file: {}", e);
            }
        },
    }
}

fn dump_to_file(dir: impl AsRef<Path>, data: &[u8]) -> Result<PathBuf> {
    #[inline]
    fn ensure_dir(dir: &Path) -> Result<()> {
        if !dir.exists() {
            fs::create_dir_all(dir).with_context(|| format!("create directory '{}' for dump files", dir.display()))?;
        } else if !dir.is_dir() {
            return Err(anyhow!("{} is not directory", dir.display()));
        }
        Ok(())
    }

    let dir = dir.as_ref();
    ensure_dir(dir)?;
    let filename = format!("{}.json", Uuid::new_v4().as_simple());
    let path = dir.join(&filename);
    fs::write(&path, data)?;
    Ok(path)
}

pub struct Hooks<P, F> {
    pub prepare: Option<P>,
    pub finalize: Option<F>,
}

/// Builder for Producer
pub struct ProducerBuilder<HP, HF> {
    bin: PathBuf,
    args: Vec<String>,
    envs: HashMap<String, String>,

    inherit_envs: bool,
    stable_timeout: Option<Duration>,
    hooks: Hooks<HP, HF>,
}

impl<HP, HF> ProducerBuilder<HP, HF> {
    /// Construct a new builder with the given binary path & args.
    pub fn new(bin: PathBuf, args: Vec<String>) -> Self {
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
    pub fn inherit_envs(mut self, yes: bool) -> Self {
        self.inherit_envs = yes;
        self
    }

    /// Set a pair of env name & value for the child.
    pub fn env(mut self, name: String, value: String) -> Self {
        self.envs.insert(name, value);
        self
    }

    /// Set the timeout before we wait for the child process to be stable, default is None, thus we
    /// would block util the child gives the ready message.
    pub fn stable_timeout(mut self, timeout: Duration) -> Self {
        self.stable_timeout.replace(timeout);
        self
    }

    /// Set if the child has a preferred numa node.
    #[cfg(feature = "numa")]
    pub fn numa_preferred(self, node: std::os::raw::c_int) -> Self {
        self.env(crate::sys::numa::ENV_NUMA_PREFERRED.to_string(), node.to_string())
    }

    /// Set a prepare hook, which will be called before the Producer send the task to the child.
    pub fn hook_prepare(mut self, f: HP) -> Self {
        self.hooks.prepare.replace(f);
        self
    }

    /// Set a finalize hook, which will be called before the Processor::process returns.
    pub fn hook_finalize(mut self, f: HF) -> Self {
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

        info!("producer ready");

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
        let pid = self.child_pid();
        thread::spawn(move || {
            let _span = warn_span!("response handler");
            if let Err(e) = start_response_handler::<T>(pid, stdout, out_txes) {
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

/// Could be use to avoid type annotations problem
pub type BoxedPrepareHook<T> = Box<dyn Fn(&Request<T>) -> Result<()> + Send + Sync>;

/// Could be use to avoid type annotations problem
pub type BoxedFinalizeHook<T> = Box<dyn Fn(&Request<T>) + Send + Sync>;

impl<T, HP, HF> Processor<T> for Producer<T, HP, HF>
where
    T: Task,
    HP: Fn(&Request<T>) -> Result<()> + Send + Sync,
    HF: Fn(&Request<T>) + Send + Sync,
{
    fn process(&self, task: T) -> Result<T::Output> {
        let req = Request {
            id: self.id.fetch_add(1, Ordering::Relaxed),
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

        let mut output = output_rx.recv().map_err(|_| anyhow!("output channel broken"))?;
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

#[cfg(test)]
mod tests {
    use std::fs;

    use pretty_assertions::assert_eq;
    use tracing_test::traced_test;

    use super::{dump, dump_to_file, DumpType};

    fn dummy_serde_err() -> serde_json::Error {
        serde_json::from_str::<Vec<u8>>("DUMMY").unwrap_err()
    }

    #[test]
    #[traced_test]
    fn test_dump_to_log() {
        let cases = vec![
            ("abcdefg".to_string(), format!("'{}'", "abcdefg")),
            ("一二三四五123".to_string(), format!("'{}'", "一二三四五123")),
            ("a".repeat(100), format!("'{}'", "a".repeat(100))),
            ("a".repeat(101), format!("'{}...'", "a".repeat(100))),
            ("a".repeat(200), format!("'{}...'", "a".repeat(100))),
            ("💗".repeat(100), format!("'{}...'", "💗".repeat(25))),
        ];

        for (data, expected_log) in cases {
            dump(&DumpType::ToLog, dummy_serde_err(), &data);
            assert!(logs_contain(&expected_log));
        }
    }

    #[test]
    fn test_dump_to_file() {
        use tempfile::tempdir;

        let tmpdir = tempdir().expect("couldn't create temp dir");

        let dumpfile = dump_to_file(tmpdir.path(), "hello world".as_bytes());
        assert!(dumpfile.is_ok(), "dump_to_file: {:?}", dumpfile.err());
        let dumpfile = dumpfile.unwrap();
        assert_eq!(tmpdir.path(), dumpfile.parent().unwrap());
        assert_eq!("hello world", fs::read_to_string(dumpfile).unwrap());
    }

    #[test]
    fn test_dump_to_file_when_dir_not_exist() {
        use tempfile::tempdir;

        let tmpdir = tempdir().expect("couldn't create temp dir");
        let not_exist_dir = tmpdir.path().join("test");

        let dumpfile = dump_to_file(&not_exist_dir, "hello world".as_bytes());
        assert!(dumpfile.is_ok(), "dump_to_file: {:?}", dumpfile.err());
        let dumpfile = dumpfile.unwrap();
        assert_eq!(not_exist_dir, dumpfile.parent().unwrap());
        assert_eq!("hello world", fs::read_to_string(dumpfile).unwrap());
    }

    #[test]
    fn test_dump_to_file_when_not_dir() {
        use tempfile::tempdir;
        let tmpdir = tempdir().expect("couldn't create temp dir");
        let tmpfile = tmpdir.path().join("test.json");
        fs::write(&tmpfile, "oops").unwrap();

        assert!(dump_to_file(&tmpfile, "hello world".as_bytes()).is_err());
    }
}
