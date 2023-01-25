//! Built-in tasks & processors

#[cfg(feature = "builtin-tasks")]
#[allow(missing_docs)]
pub mod tasks;

// #[cfg(feature = "builtin-executor")]
#[allow(missing_docs)]
pub mod executor;

/// TODO: doc
pub mod simple_processor;

/// TODO: doc
pub mod task_id_extractor;
