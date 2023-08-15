//! This module provides wrappers to construct task producer & consumer.
//! The producer sends tasks via stdin of the consumer, which should be a
//! sub-process.

use serde::{Deserialize, Serialize};

#[cfg(feature = "ext-producer")]
mod producer;
#[cfg(feature = "ext-producer")]
pub use producer::{dump_error_resp_env, Producer, ProducerBuilder};

mod consumer;
pub use consumer::{run as run_consumer, run_with_processor as run_consumer_with_proc};

/// Request contains the required data to be sent to the consumer.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Request<T> {
    /// request id which should be maintained by the producer and used later to dispatch the response
    pub id: u64,

    /// the task itself
    pub task: T,
}

/// Response contains the output for the specific task, and error message if exists.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Response<O> {
    /// request id
    pub id: u64,

    /// error message if the task execution failed
    pub err_msg: Option<String>,

    /// output of the succeeded task execution
    pub output: Option<O>,
}

#[inline]
fn ready_msg(name: &str) -> String {
    format!("{} processor ready", name)
}
