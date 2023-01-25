use std::{env::args, iter::repeat, time::Duration};

use anyhow::Context;
use crossterm::tty::IsTty;
use time::format_description::well_known::Rfc3339;
use tokio::net::TcpStream;
use tokio_util::codec::LengthDelimitedCodec;
use tracing_subscriber::{
    filter,
    fmt::{layer, time::OffsetTime},
    prelude::*,
    registry,
};
use vc_ipc::{
    client::Client,
    framed::FramedExt,
    readwriter::{reconnect::connect as reconnect, ReadWriterExt},
    serded::Bincode,
};

/// initiate the global tracing subscriber
pub fn init_log() -> anyhow::Result<()> {
    let env_filter = filter::EnvFilter::builder()
        .with_default_directive(filter::LevelFilter::INFO.into())
        .from_env()
        .context("invalid env filter")?
        .add_directive("mio=warn".parse()?)
        .add_directive("rustls=warn".parse()?)
        .add_directive("tarpc=warn".parse()?)
        .add_directive("tokio_util::codec=warn".parse()?);

    let fmt_layer = layer()
        .with_writer(std::io::stderr)
        .with_ansi(std::io::stderr().is_tty())
        .with_target(true)
        .with_thread_ids(true)
        .with_timer(OffsetTime::local_rfc_3339().unwrap_or_else(|_| OffsetTime::new(time::UtcOffset::UTC, Rfc3339)))
        .with_filter(env_filter);

    registry().with(fmt_layer).init();

    Ok(())
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    init_log()?;
    let n: usize = args().nth(1).and_then(|x| x.parse().ok()).unwrap_or(5);

    let tp = reconnect::<TcpStream, _, _>("127.0.0.1:8964", repeat(Duration::from_secs(1)))
        .framed(LengthDelimitedCodec::default())
        .serded(Bincode::default());

    let client = Client::new(Default::default(), tp).spawn();

    loop {
        let resp: Result<u64, _> = client.call(n).await;
        println!("fib({}) = {:?}", n, resp);
        tokio::time::sleep(Duration::from_secs(1)).await;
    }
}
