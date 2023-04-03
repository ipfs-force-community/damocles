//! Logging for venus-worker.

use anyhow::{Context, Result};
use opentelemetry::global;
use opentelemetry::sdk::propagation::TraceContextPropagator;
use tracing_subscriber::{
    filter::{self, FilterExt},
    fmt::{layer, time::LocalTime},
    prelude::*,
    registry, EnvFilter,
};

/// initiate the global tracing subscriber
pub fn init(service_name: &str) -> Result<()> {
    let subscriber = registry();

    let env_filter = filter::EnvFilter::builder()
        .with_default_directive(filter::LevelFilter::DEBUG.into())
        .from_env()
        .context("invalid env filter")?
        .add_directive("want=warn".parse()?)
        .add_directive("jsonrpc_client_transports=warn".parse()?)
        .add_directive("jsonrpc_core=warn".parse()?)
        .add_directive("hyper=warn".parse()?)
        .add_directive("mio=warn".parse()?)
        .add_directive("reqwest=warn".parse()?)
        .add_directive("tokio_util=warn".parse()?);

    let worker_env_filter = filter::EnvFilter::builder()
        .with_default_directive(filter::LevelFilter::OFF.into())
        .with_env_var("VENUS_WORKER_LOG")
        .from_env()
        .context("invalid venus worker log filter")?;

    let fmt_layer = layer()
        .with_writer(std::io::stderr)
        .with_ansi(false)
        .with_target(true)
        .with_thread_ids(true)
        .with_timer(LocalTime::rfc_3339())
        .with_filter(env_filter.or(worker_env_filter));

    let subscriber = subscriber.with(fmt_layer);

    // Jaeger layer.
    let mut jaeger_layer = None;
    let jaeger_agent_endpoint = std::env::var("JAEGER_AGENT_ENDPOINT").unwrap_or_else(|_| "".to_string());
    if !jaeger_agent_endpoint.is_empty() {
        global::set_text_map_propagator(TraceContextPropagator::new());

        let tracer = opentelemetry_jaeger::new_agent_pipeline()
            .with_service_name(service_name)
            .with_endpoint(jaeger_agent_endpoint)
            // .with_auto_split_batch(true)
            .install_batch(opentelemetry::runtime::Tokio)
            .expect("install");

        // Load filter from `RUST_LOG`. Default to `ERROR`.
        let env_filter = EnvFilter::from_default_env();
        jaeger_layer = Some(tracing_opentelemetry::layer().with_tracer(tracer).with_filter(env_filter));
    }
    let subscriber = subscriber.with(jaeger_layer);

    // tokio-console layer
    #[cfg(feature = "console")]
    let subscriber = subscriber.with(console_subscriber::spawn());

    subscriber.try_init().context("try init tracing subscriber")?;
    Ok(())
}
