//! provides logging helpers

use anyhow::Result;
use crossterm::tty::IsTty;
use tracing::subscriber::set_global_default;
use tracing_subscriber::{fmt::time, EnvFilter, FmtSubscriber};

pub use tracing::{
    debug, debug_span, error, error_span, field::debug as debug_field,
    field::display as display_field, info, info_span, trace, trace_span, warn, warn_span, Span,
};

/// initiaate the global tracing subscriber
pub fn init() -> Result<()> {
    let builder = FmtSubscriber::builder()
        .with_ansi(std::io::stdout().is_tty() && std::io::stderr().is_tty())
        .with_env_filter(EnvFilter::from_default_env())
        .with_target(true)
        .with_thread_ids(true)
        .with_timer(time::ChronoLocal::rfc3339());

    set_global_default(builder.finish())?;
    Ok(())
}
