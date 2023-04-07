//! Tower middleware

pub mod limit;

/// An error indicating that the processor with a `K`-typed key failed with an
/// error.
pub type BoxError = Box<dyn std::error::Error + Send + Sync>;
