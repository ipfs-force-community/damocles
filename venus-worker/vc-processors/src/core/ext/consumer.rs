use std::io::{stdin, stdout, Write};

use anyhow::{anyhow, Context, Result};
use serde_json::{from_str, to_string};
use tracing::{debug, error, info, warn_span};

use super::{ready_msg, Request, Response};
use crate::core::{Processor, Task};

/// Starts the consumer with the given processor.
/// In most cases, this is used in a sub-process
/// Please be awrae that you should let the logs output into stderr, or just disable all logs
/// from this process, otherwise the producer colud be confused.
pub fn run_with_processor<T: Task, P: Processor<T> + Send + Sync + Clone + 'static>(proc: P) -> Result<()> {
    #[cfg(feature = "numa")]
    crate::sys::numa::try_set_preferred();

    let _span = warn_span!("sub", name = %T::STAGE, pid = std::process::id()).entered();

    let mut output = stdout();
    writeln!(output, "{}", ready_msg(T::STAGE)).context("write ready msg")?;

    let input = stdin();
    let mut line = String::new();

    info!("processor ready");
    loop {
        debug!("waiting for new incoming line");
        line.clear();
        let size = input.read_line(&mut line)?;
        if size == 0 {
            return Err(anyhow!("got empty line, parent might be out"));
        }

        let req: Request<T> = match from_str(&line).context("unmarshal request") {
            Ok(r) => r,
            Err(e) => {
                error!("unmarshal request: {:?}", e);
                continue;
            }
        };

        let cloned = proc.clone();
        let req_span = warn_span!("request", id = req.id, size = size);
        std::thread::spawn(move || {
            req_span.in_scope(|| {
                if let Err(e) = process_request(cloned, req) {
                    error!("failed: {:?}", e);
                }
            })
        });
    }
}

/// Starts the consumer with the processor impl with Default.
pub fn run<T: Task, P: Processor<T> + Default + Send + Sync + Copy + 'static>() -> Result<()> {
    let proc = P::default();

    run_with_processor(proc)
}

fn process_request<T: Task, P: Processor<T>>(proc: P, req: Request<T>) -> Result<()> {
    debug!("request received");

    let resp = match proc.process(req.task) {
        Ok(out) => Response {
            id: req.id,
            err_msg: None,
            output: Some(out),
        },

        Err(e) => Response {
            id: req.id,
            err_msg: Some(format!("{:?}", e)),
            output: None,
        },
    };
    debug!(ok = resp.output.is_some(), "request done");

    let res_str = to_string(&resp).context("marshal response")?;
    let sout = stdout();
    let mut output = sout.lock();
    writeln!(output, "{}", res_str).context("write output")?;
    drop(output);

    debug!("response written");

    Ok(())
}
