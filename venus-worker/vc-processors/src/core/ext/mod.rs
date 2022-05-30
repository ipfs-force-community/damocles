//! This module provides wrappers to construct task producer & consumer.
//! The producer sends tasks to the consumer via stdin of the consumer, which should be a
//! sub-process.

use serde::{Deserialize, Serialize};

mod sub;
pub use sub::subrun;

#[derive(Clone, Debug, Serialize, Deserialize)]
struct Request<T> {
    pub id: u64,
    pub data: T,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
struct Response<T> {
    pub id: u64,
    pub err_msg: Option<String>,
    pub result: Option<T>,
}

#[inline]
fn ready_msg(name: &str) -> String {
    format!("{} processor ready", name)
}
