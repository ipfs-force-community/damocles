use anyhow::{anyhow, Context, Result};
use clap::{value_t, App, AppSettings, Arg, SubCommand};
use tokio::runtime::Builder;

mod generator;
mod processor;
mod store;
mod worker;

pub fn main() -> Result<()> {
    let rt = Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("construct tokio runtime")?;

    let _rt_guard = rt.enter();

    let daemon_cmd = SubCommand::with_name("daemon")
        .arg(Arg::with_name("api").long("api").takes_value(true).help("sealer api addr"))
        .arg(
            Arg::with_name("config")
                .long("config")
                .short("c")
                .takes_value(true)
                .help("path for the config file"),
        );

    let generator_cmd = generator::subcommand();
    let processor_cmd = processor::subcommand();
    let store_cmd = store::subcommand();
    let worker_cmd = worker::subcommand();

    let ver_string = format!("v{}-{}", env!("CARGO_PKG_VERSION"), option_env!("GIT_COMMIT").unwrap_or("dev"));

    let app = App::new("vc-worker")
        .version(ver_string.as_str())
        .setting(AppSettings::ArgRequiredElseHelp)
        .subcommand(daemon_cmd)
        .subcommand(generator_cmd)
        .subcommand(processor_cmd)
        .subcommand(store_cmd)
        .subcommand(worker_cmd);

    let matches = app.get_matches();

    match matches.subcommand() {
        ("daemon", Some(m)) => {
            let cfg_path = value_t!(m, "config", String)?;
            venus_worker::tracing::logging::init("venus-worker")?;
            venus_worker::start_daemon(cfg_path)
        }

        (generator::SUB_CMD_NAME, Some(args)) => generator::submatch(args),

        (processor::SUB_CMD_NAME, Some(args)) => processor::submatch(args),

        (store::SUB_CMD_NAME, Some(args)) => store::submatch(args),

        (worker::SUB_CMD_NAME, Some(args)) => worker::submatch(args),

        (name, _) => Err(anyhow!("unexpected subcommand `{}`", name)),
    }
}
