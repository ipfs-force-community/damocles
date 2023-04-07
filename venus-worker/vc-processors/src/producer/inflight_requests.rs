use std::{
    task::{Context, Poll},
    time::SystemTime,
};

use fnv::FnvHashMap;
use tokio::sync::oneshot;
use tokio_util::time::{delay_queue, DelayQueue};

use crate::{Request, Response, Task};

/// The request exceeded its deadline.
#[derive(thiserror::Error, Debug)]
#[non_exhaustive]
#[error("the request exceeded its deadline")]
pub struct DeadlineExceededError;

#[derive(Debug)]
struct RequestData<Tsk: Task, TskId> {
    completion: oneshot::Sender<Result<Response<Tsk::Output, TskId>, DeadlineExceededError>>,
    /// The key to remove the timer for the request's deadline.
    deadline_key: delay_queue::Key,
    span: tracing::Span,
}

#[derive(Debug)]
pub(crate) struct InflightRequests<Tsk: Task, TskId> {
    request_data: FnvHashMap<u64, RequestData<Tsk, TskId>>,
    deadlines: DelayQueue<u64>,
}

impl<Tsk: Task, TskId> InflightRequests<Tsk, TskId> {
    pub fn new() -> Self {
        Self {
            request_data: Default::default(),
            deadlines: Default::default(),
        }
    }

    pub fn send(
        &mut self,
        request: &Request<Tsk, TskId>,
        response_tx: oneshot::Sender<Result<Response<Tsk::Output, TskId>, DeadlineExceededError>>,
        span: tracing::Span,
    ) {
        tracing::debug!(request_id = request.request_id, "insert request");
        let timeout = request.context.deadline().duration_since(SystemTime::now()).unwrap_or_default();
        let deadline_key = self.deadlines.insert(request.request_id, timeout);
        self.request_data.insert(
            request.request_id,
            RequestData {
                completion: response_tx,
                deadline_key,
                span,
            },
        );
    }

    /// complete represents the specified response is ready
    pub fn complete(&mut self, response: Response<Tsk::Output, TskId>) -> bool {
        if let Some(RequestData {
            completion,
            deadline_key,
            span,
        }) = self.request_data.remove(&response.request_id)
        {
            let _entered = span.enter();
            tracing::debug!(request_id = response.request_id, "receive response");
            let _ = completion.send(Ok(response));
            self.request_data.shrink_to_fit();
            if self.deadlines.try_remove(&deadline_key).is_some() {
                self.deadlines.shrink_to_fit();
            }
            return true;
        }

        tracing::warn!("no in-flight request found for request_id = {}.", response.request_id);

        false
    }

    /// Cancels a request without completing
    pub fn cancel(&mut self, request_id: u64) -> Option<tracing::Span> {
        self.request_data.remove(&request_id).map(|RequestData { deadline_key, span, .. }| {
            span.in_scope(|| {
                tracing::debug!(request_id = request_id, "request was canceled");
            });
            self.request_data.shrink_to_fit();
            if self.deadlines.try_remove(&deadline_key).is_some() {
                self.deadlines.shrink_to_fit();
            }
            span
        })
    }

    /// Yields a request that has expired, completing it with a TimedOut error.
    /// The caller should send cancellation messages for any yielded request ID.
    pub fn poll_expired(&mut self, cx: &mut Context) -> Poll<Option<u64>> {
        self.deadlines.poll_expired(cx).map(|expired| {
            let request_id = expired?.into_inner();
            if let Some(request_data) = self.request_data.remove(&request_id) {
                let _entered = request_data.span.enter();
                tracing::error!(request_id = request_id, "DeadlineExceeded");
                self.request_data.shrink_to_fit();

                let _ = request_data.completion.send(Err(DeadlineExceededError));
            }
            Some(request_id)
        })
    }
}
