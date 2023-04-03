//! Provides request types
//!

use serde::{Deserialize, Serialize};

use crate::context::Context;

/// Request contains the required data to be sent to the consumer.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Request<Tsk, TskId> {
    /// request id which should be maintained by the producer and used later to dispatch the response
    pub request_id: u64,
    /// Trace context, deadline, and other cross-cutting concerns.
    pub context: Context,
    /// The request body.
    pub body: RequestBody<Tsk, TskId>,
}

/// The request body type
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "method")]
pub enum RequestBody<Tsk, TskId> {
    /// CancelRequest means canceling a specified request.
    CancelRequest,
    /// StartTask means start the specified task.
    StartTask { task: Tsk },
    /// WaitTask means start the specified task.
    WaitTask { task_id: TskId },
}

impl<Tsk, TskId> Request<Tsk, TskId> {
    /// Returns a new request
    pub fn new(request_id: u64, body: RequestBody<Tsk, TskId>) -> Self {
        Self {
            request_id,
            context: Context::current(),
            body,
        }
    }

    /// Returns a new cancel request
    pub fn new_cancel_request(request_id: u64, origin_span: tracing::Span) -> Self {
        Self {
            request_id,
            context: (&origin_span).into(),
            body: RequestBody::CancelRequest,
        }
    }
    /// Returns true if the request is a cancel request
    pub fn is_cancel_request(&self) -> bool {
        match self.body {
            RequestBody::CancelRequest => true,
            _ => false,
        }
    }
}
