use std::io::{stdin, stdout, Write};

use anyhow::{anyhow, Context, Result};
use serde_json::{from_str, to_string};

use super::{
    super::{C2Input, Input, PC1Input, PC2Input, TreeDInput},
    ready_msg, Request, Response,
};
use crate::logging::{debug, error, info, warn_span};

/// start the main loop of c2 processor
pub fn run_tree_d() -> Result<()> {
    run::<TreeDInput>()
}

/// start the main loop of pc1 processor
pub fn run_pc1() -> Result<()> {
    run::<PC1Input>()
}

/// start the main loop of pc2 processor
pub fn run_pc2() -> Result<()> {
    run::<PC2Input>()
}

/// start the main loop of c2 processor
pub fn run_c2() -> Result<()> {
    run::<C2Input>()
}

/// used for processor sub command
pub fn run<I: Input>() -> Result<()> {
    #[cfg(feature = "numa")]
    crate::sys::numa::try_set_preferred();

    let name = I::STAGE.name();

    let mut output = stdout();
    writeln!(output, "{}", ready_msg(name))?;

    let pid = std::process::id();
    let span = warn_span!("sub", name, pid);
    let _guard = span.enter();

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

        let req: Request<I> = match from_str(&line).context("unmarshal request") {
            Ok(r) => r,
            Err(e) => {
                error!("unmarshal request: {:?}", e);
                continue;
            }
        };

        std::thread::spawn(move || {
            let _guard = warn_span!("request", id = req.id, size = size).entered();
            if let Err(e) = process_request(req) {
                error!("failed: {:?}", e);
            }
        });
    }
}

fn process_request<I: Input>(req: Request<I>) -> Result<()> {
    debug!("request received");

    let resp = match req.data.process() {
        Ok(out) => Response {
            id: req.id,
            err_msg: None,
            result: Some(out),
        },

        Err(e) => Response {
            id: req.id,
            err_msg: Some(format!("{:?}", e)),
            result: None,
        },
    };
    debug!(ok = resp.result.is_some(), "request done");

    let res_str = to_string(&resp).context("marshal response")?;
    let sout = stdout();
    let mut output = sout.lock();
    writeln!(output, "{}", res_str)?;
    drop(output);

    debug!("response written");

    Ok(())
}
