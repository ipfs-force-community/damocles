use std::collections::HashMap;
use std::env::{self, vars};
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::sync::{
    atomic::{AtomicU64, Ordering},
    Arc, Mutex,
};
use std::time::Duration;
use std::{fs, io, thread};

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{after, bounded, never, select, Receiver, Sender};
use fnv::FnvHashMap;
use serde_json::{from_str, to_string};
use tracing::{debug, error, info};
use uuid::Uuid;

use super::{ready_msg, Request, Response};
use crate::core::{Processor, Task};

/// Dump error resp env key
pub fn dump_error_resp_env(pid: u32) -> String {
    format!("DUMP_ERR_RESP_{}", pid)
}

fn start_response_handler<T: Task>(child_pid: u32, stdout: ChildStdout, in_flight_requests: InflightRequests<T::Output>) -> Result<()> {
    let mut reader = BufReader::new(stdout);
    let mut line_buf = String::new();

    loop {
        line_buf.clear();

        let size = reader.read_line(&mut line_buf).context("read line from stdout")?;
        if size == 0 {
            error!("child exited");
            return Err(io::Error::new(io::ErrorKind::BrokenPipe, "child process exit").into());
        }

        let resp: Response<T::Output> = match from_str(line_buf.as_str()) {
            Ok(r) => r,
            Err(_) => {
                dump(&DumpType::from_env(child_pid), child_pid, line_buf.as_str());
                continue;
            }
        };

        debug!(id = resp.id, size, "response received");
        // this should not be blocked
        in_flight_requests.complete(resp);
    }
}

/// Dump type of the error format response of the processor
#[derive(Debug, Clone)]
enum DumpType {
    /// Dump error format response fragments to the log
    ToLog,
    /// Dump error format response to the file
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

fn dump(dt: &DumpType, child_pid: u32, data: &str) {
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
            error!(child_pid = child_pid, "failed to unmarshal response string: '{}'.", truncate(data));
        }
        DumpType::ToFile(dir) => match dump_to_file(child_pid, dir, data.as_bytes()) {
            Ok(dump_file_path) => {
                error!(
                    child_pid = child_pid,
                    "failed to unmarshal response string. dump file '{}' generated.",
                    dump_file_path.display()
                )
            }
            Err(e) => {
                error!(
                    child_pid = child_pid,
                    "failed to unmarshal response string; failed to generate dump file: '{}'.", e
                );
            }
        },
    }
}

fn dump_to_file(child_pid: u32, dir: impl AsRef<Path>, data: &[u8]) -> Result<PathBuf> {
    #[inline]
    fn ensure_dir(dir: &Path) -> Result<()> {
        if !dir.exists() {
            fs::create_dir_all(dir).with_context(|| format!("create directory '{}' for dump files", dir.display()))?;
        } else if !dir.is_dir() {
            return Err(anyhow!("'{}' is not directory", dir.display()));
        }
        Ok(())
    }

    let dir = dir.as_ref();
    ensure_dir(dir)?;
    let filename = format!("ext-processor-err-resp-{}-{}.json", child_pid, Uuid::new_v4().as_simple());
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
    auto_restart: bool,
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
            auto_restart: false,
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

    /// Set auto restart child process
    pub fn auto_restart(mut self, auto_restart: bool) -> Self {
        self.auto_restart = auto_restart;
        self
    }

    /// Build a Producer with the given options.
    pub fn spawn<T: Task>(self) -> Result<Producer<T, HP, HF>> {
        let ProducerBuilder {
            bin,
            args,
            mut envs,
            inherit_envs,
            stable_timeout,
            hooks,
            auto_restart,
        } = self;

        if inherit_envs {
            envs.extend(vars());
        }

        let cmd = move || {
            let mut cmd = Command::new(&bin);
            cmd.args(args.clone())
                .envs(envs.clone())
                .stdin(Stdio::piped())
                .stdout(Stdio::piped())
                .stderr(Stdio::inherit());
            cmd
        };

        let (producer_inner, mut child_stdout) = ProducerInner::new(cmd(), T::STAGE, stable_timeout).context("create producer inner")?;
        let child_pid = producer_inner.child_id();
        let producer_inner = Arc::new(Mutex::new(producer_inner));
        let in_flight_requests = InflightRequests::new();

        let producer = Producer {
            next_id: AtomicU64::new(1),
            inner: producer_inner.clone(),
            hooks,
            in_flight_requests: in_flight_requests.clone(),
        };

        thread::spawn(move || {
            loop {
                if let Err(e) =
                    start_response_handler::<T>(child_pid, child_stdout, in_flight_requests.clone()).context("start response handler")
                {
                    error!(err=?e, "failed to start response handler. pid: {}", child_pid);
                }

                // child process exist
                // cancel all in flight requests
                in_flight_requests.cancel_all(format!("child process exited: {}", T::STAGE));

                if !auto_restart {
                    break;
                }
                thread::sleep(Duration::from_secs(3));

                let mut inner = producer_inner.lock().unwrap();
                match inner.restart_child(cmd(), T::STAGE, stable_timeout) {
                    Ok(new_child_stdout) => {
                        child_stdout = new_child_stdout;
                    }
                    Err(e) => {
                        error!(err=?e, "unable to restart child process");
                        break;
                    }
                }
            }
        });

        Ok(producer)
    }
}

fn wait_for_stable(stage: &'static str, stdout: ChildStdout, mut stable_timeout: Option<Duration>) -> Result<ChildStdout> {
    fn inner(res_tx: Sender<Result<()>>, stage: &'static str, stdout: ChildStdout) -> thread::JoinHandle<ChildStdout> {
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

    let (stable_tx, stable_rx) = bounded(0);
    let stable_hdl = inner(stable_tx, stage, stdout);
    let wait = stable_timeout.take().map(after).unwrap_or_else(never);

    select! {
        recv(stable_rx) -> ready_res => {
            ready_res.context("stable chan broken")?.context("wait for stable")?;
            info!("producer ready");
            let stdout = stable_hdl.join().map_err(|_| anyhow!("wait for stable handle to be joined"))?;
            Ok(stdout)
        },

        recv(wait) -> _ => {
            Err(anyhow!("timeout exceeded before child get ready"))
        }
    }
}

/// Producer sends tasks to the child, and waits for the responses.
/// It impl Processor.
pub struct Producer<T: Task, HP, HF> {
    next_id: AtomicU64,
    inner: Arc<Mutex<ProducerInner>>,
    hooks: Hooks<HP, HF>,
    in_flight_requests: InflightRequests<T::Output>,
}

impl<T, HP, HF> Producer<T, HP, HF>
where
    T: Task,
{
    /// Returns the child's process id.
    pub fn child_pid(&self) -> u32 {
        self.inner.lock().unwrap().child_id()
    }

    /// Returns the child's process id.
    pub fn next_id(&self) -> u64 {
        self.next_id.fetch_add(1, Ordering::Relaxed)
    }

    fn send(&self, req: &Request<T>) -> Result<()> {
        let data = to_string(req).context("marshal request")?;
        self.inner.lock().unwrap().write_data(data)
    }
}

struct ProducerInner {
    child: Child,
    child_stdin: ChildStdin,
}

impl ProducerInner {
    fn new(cmd: Command, stage: &'static str, stable_timeout: Option<Duration>) -> Result<(Self, ChildStdout)> {
        let (child, child_stdin, child_stdout) = Self::create_child_and_wait_it(cmd, stage, stable_timeout)?;
        Ok((Self { child, child_stdin }, child_stdout))
    }

    fn child_id(&self) -> u32 {
        self.child.id()
    }

    fn write_data(&mut self, data: String) -> Result<()> {
        writeln!(self.child_stdin, "{}", data).context("write request data")
        // TODO: flush?
    }

    /// restart_child restarts the child process
    fn restart_child(&mut self, cmd: Command, stage: &'static str, stable_timeout: Option<Duration>) -> Result<ChildStdout> {
        info!("restart the child process: {:?}", cmd);
        self.kill_child();

        let (child, child_stdin, child_stdout) = Self::create_child_and_wait_it(cmd, stage, stable_timeout)?;
        self.child = child;
        self.child_stdin = child_stdin;

        Ok(child_stdout)
    }

    fn create_child_and_wait_it(
        mut cmd: Command,
        stage: &'static str,
        stable_timeout: Option<Duration>,
    ) -> Result<(Child, ChildStdin, ChildStdout)> {
        let mut child = cmd.spawn().context("spawn child process")?;
        let child_stdin = child.stdin.take().context("child stdin lost")?;
        let mut child_stdout = child.stdout.take().context("child stdout lost")?;
        child_stdout = wait_for_stable(stage, child_stdout, stable_timeout).context("wait for child process stable")?;
        Ok((child, child_stdin, child_stdout))
    }

    fn kill_child(&mut self) {
        info!(pid = self.child.id(), "kill child");
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

impl Drop for ProducerInner {
    fn drop(&mut self) {
        self.kill_child()
    }
}

#[derive(Debug, Default)]
struct InflightRequests<O>(Arc<Mutex<FnvHashMap<u64, Sender<Response<O>>>>>);

impl<O> InflightRequests<O> {
    pub fn new() -> Self {
        Self(Default::default())
    }

    /// sent represents the specified request data has sent to the target.
    pub fn sent(&self, id: u64) -> Receiver<Response<O>> {
        debug!("sent request: {}", id);
        let (tx, rx) = bounded(0);
        self.0.lock().unwrap().insert(id, tx);
        rx
    }

    pub fn remove(&self, id: u64) {
        self.0.lock().unwrap().remove(&id);
    }

    /// complete represents the specified response is ready
    pub fn complete(&self, resp: Response<O>) {
        if let Some(tx) = self.0.lock().unwrap().remove(&resp.id) {
            let _ = tx.send(resp);
        }
    }

    pub fn cancel_all(&self, err_msg: impl AsRef<str>) {
        let mut inner = self.0.lock().unwrap();
        let keys: Vec<_> = inner.keys().cloned().collect();
        for id in keys {
            if let Some(tx) = inner.remove(&id) {
                let _ = tx.send(Response {
                    id,
                    err_msg: Some(err_msg.as_ref().to_string()),
                    output: None,
                });
                debug!("canceled request: {}", id);
            }
        }
    }
}

impl<T> Clone for InflightRequests<T> {
    fn clone(&self) -> Self {
        Self(self.0.clone())
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
        let req = Request { id: self.next_id(), task };

        if let Some(p) = self.hooks.prepare.as_ref() {
            p(&req).context("prepare task")?;
        };

        let _defer = Defer(true, || {
            if let Some(f) = self.hooks.finalize.as_ref() {
                f(&req);
            }
        });

        let rx = self.in_flight_requests.sent(req.id);
        if let Err(e) = self.send(&req) {
            self.in_flight_requests.remove(req.id);
            return Err(e);
        }

        debug!("wait request: {}", req.id);
        let mut output = rx.recv().map_err(|_| anyhow!("output channel broken"))?;
        if let Some(err_msg) = output.err_msg.take() {
            return Err(anyhow!(err_msg));
        }
        debug!("request done: {}", req.id);
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
            dump(&DumpType::ToLog, 1, &data);
            assert!(logs_contain(&expected_log));
        }
    }

    #[test]
    fn test_dump_to_file() {
        use tempfile::tempdir;

        let tmpdir = tempdir().expect("couldn't create temp dir");

        let dumpfile = dump_to_file(1, tmpdir.path(), "hello world".as_bytes());
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

        let dumpfile = dump_to_file(1, &not_exist_dir, "hello world".as_bytes());
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

        assert!(dump_to_file(1, &tmpfile, "hello world".as_bytes()).is_err());
    }
}
