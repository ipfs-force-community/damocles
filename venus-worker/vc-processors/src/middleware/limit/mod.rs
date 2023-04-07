//! Tower middleware for limiting requests.
//!

/// delays sending the request to the underlying service
pub mod delay;

/// limit requests based on the given locks
pub mod lock;
