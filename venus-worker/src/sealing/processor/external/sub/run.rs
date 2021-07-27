use std::io::{stdin, stdout, Write};

use anyhow::Result;
use serde_json::{from_str, to_string};

use super::{
    super::{C2Input, Input, PC2Input},
    ready_msg, Response,
};
use crate::logging::{debug, info, info_span, trace};

/// start the main loop of pc2 processor
pub fn run_pc2() -> Result<()> {
    run::<PC2Input>()
}

/// start the main loop of c2 processor
pub fn run_c2() -> Result<()> {
    run::<C2Input>()
}

/// used for processor sub command
fn run<I: Input>() -> Result<()> {
    let name = I::STAGE.name();

    let mut output = stdout();
    writeln!(output, "{}", ready_msg(name))?;

    let pid = std::process::id();
    let span = info_span!("sub", name, pid);
    let _guard = span.enter();

    let mut line = String::new();
    let input = stdin();

    info!("processor ready");
    loop {
        debug!("waiting for new incoming line");
        input.read_line(&mut line)?;
        trace!("line: {}", line.as_str());

        debug!("process line");
        let response = match process_line::<I>(line.as_str()) {
            Ok(o) => Response {
                err_msg: None,
                result: Some(o),
            },

            Err(e) => Response {
                err_msg: Some(format!("{:?}", e)),
                result: None,
            },
        };
        trace!("response: {:?}", response);

        debug!("write output");
        let res_str = to_string(&response)?;
        trace!("response: {}", res_str.as_str());
        writeln!(output, "{}", res_str)?;
        line.clear();
    }
}

fn process_line<I: Input>(line: &str) -> Result<I::Out> {
    let input: I = from_str(line)?;
    trace!("input: {:?}", input);

    input.process()
}
