//! provides logging helpers

use std::io::{Error, Write};

use ansi_term::{Colour, Style};
use anyhow::Result;
use crossterm::tty::IsTty;
use flexi_logger::{
    DeferredNow, Level, LevelFilter, LogSpecBuilder, LogSpecification, Logger, Record,
};
use tracing::subscriber::set_global_default;
use tracing_subscriber::{fmt::time, EnvFilter, FmtSubscriber};

pub use tracing::{
    debug, debug_span, error, error_span, field::debug as debug_field,
    field::display as display_field, info, info_span, trace, trace_span, warn, warn_span, Span,
};

/// initiate the global tracing subscriber
pub fn init() -> Result<()> {
    let builder = FmtSubscriber::builder()
        .with_writer(std::io::stderr)
        .with_ansi(std::io::stderr().is_tty())
        .with_env_filter(EnvFilter::from_default_env())
        .with_target(true)
        .with_thread_ids(true)
        .with_timer(time::ChronoLocal::rfc3339());

    set_global_default(builder.finish())?;

    init_flexi_logger()
}

fn init_flexi_logger() -> Result<()> {
    let mut builder = LogSpecBuilder::new();
    builder.default(LevelFilter::Info);
    builder.module("jsonrpc_client_transports", LevelFilter::Warn);
    builder.module("jsonrpc_core", LevelFilter::Warn);
    builder.module("jsonrpc_ws_server", LevelFilter::Warn);
    builder.module("parity_ws", LevelFilter::Warn);

    let from_env = LogSpecification::env()?;
    builder.insert_modules_from(from_env);

    let logger =
        Logger::with(builder.finalize())
            .log_to_stderr()
            .format(if std::io::stderr().is_tty() {
                flexi_colored_format
            } else {
                flexi_no_color_format
            });

    logger.start()?;

    Ok(())
}

fn flexi_no_color_format(
    w: &mut dyn Write,
    now: &mut DeferredNow,
    record: &Record<'_>,
) -> Result<(), Error> {
    write!(
        w,
        "{} {} {:?} {}: {}",
        now.now().to_rfc3339(),
        record.level(),
        std::thread::current().id(),
        record.module_path().unwrap_or("<unnamed>"),
        &record.args(),
    )
}

fn flexi_colored_format(
    w: &mut dyn Write,
    now: &mut DeferredNow,
    record: &Record<'_>,
) -> Result<(), Error> {
    let level = record.level();
    write!(
        w,
        "{} {} {:?} {}: {}",
        Style::new().dimmed().paint(now.now().to_rfc3339()),
        style_for_level(level).paint(level.as_str()),
        std::thread::current().id(),
        record.module_path().unwrap_or("<unnamed>"),
        &record.args(),
    )
}

fn style_for_level(l: Level) -> Style {
    match l {
        Level::Trace => Style::new().fg(Colour::Purple),
        Level::Debug => Style::new().fg(Colour::Blue),
        Level::Info => Style::new().fg(Colour::Green),
        Level::Warn => Style::new().fg(Colour::Yellow),
        Level::Error => Style::new().fg(Colour::Red),
    }
}
