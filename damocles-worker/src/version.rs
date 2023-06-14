use once_cell::sync::Lazy;

/// Defines the application version.
pub static VERSION: Lazy<String> =
    Lazy::new(|| format!("v{}-{}", env!("CARGO_PKG_VERSION"), option_env!("GIT_COMMIT").unwrap_or("unknown")));
