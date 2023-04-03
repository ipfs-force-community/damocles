use std::{
    io,
    sync::atomic::{AtomicU64, Ordering},
};

use serde::{de::DeserializeOwned, Serialize};
use tokio::sync::{mpsc::UnboundedSender, oneshot};

use crate::{
    request::RequestBody,
    response::{ConsumerError, ResponseBody},
    Request, Response, Task,
};

use super::{dispatch::DispatchRequest, inflight_requests::DeadlineExceededError};

#[derive(Debug)]
pub(crate) struct ConsumerClient<Tsk: Task, TskId> {
    next_id: AtomicU64,
    to_dispatch_cancellation: UnboundedSender<u64>,
    to_dispatch_request: UnboundedSender<DispatchRequest<Tsk, TskId>>,
}

impl<Tsk: Task, TskId> ConsumerClient<Tsk, TskId> {
    pub fn new(to_dispatch_request: UnboundedSender<DispatchRequest<Tsk, TskId>>, to_dispatch_cancellation: UnboundedSender<u64>) -> Self {
        Self {
            next_id: AtomicU64::new(1),
            to_dispatch_request,
            to_dispatch_cancellation,
        }
    }

    pub async fn start_task(&self, task: Tsk) -> Result<TskId, Error>
    where
        Tsk: Task,
        TskId: Serialize + DeserializeOwned,
    {
        match self.request(RequestBody::StartTask { task }).await? {
            ResponseBody::StartTask { task_id } => Ok(task_id),
            ResponseBody::WaitTask { .. } => Err(Error::Consumer(ConsumerError {
                kind: io::ErrorKind::InvalidData,
                detail: "invalid method, expected method start_task".into(),
            })),
        }
    }

    pub async fn wait_task(&self, task_id: TskId) -> Result<Option<Tsk::Output>, Error>
    where
        Tsk: Task,
        TskId: Serialize + DeserializeOwned,
    {
        match self.request(RequestBody::WaitTask { task_id }).await? {
            ResponseBody::StartTask { .. } => Err(Error::Consumer(ConsumerError {
                kind: io::ErrorKind::InvalidData,
                detail: "invalid method, expected method wait_task".into(),
            })),
            ResponseBody::WaitTask { output } => Ok(output),
        }
    }

    /// Returns the next request id
    fn next_id(&self) -> u64 {
        self.next_id.fetch_add(1, Ordering::Relaxed)
    }

    async fn request(&self, body: RequestBody<Tsk, TskId>) -> Result<ResponseBody<Tsk::Output, TskId>, Error> {
        let (response_tx, mut response_rx) = oneshot::channel();
        let request_id = self.next_id();

        // to_dispatch_request is never closed
        let _ = self.to_dispatch_request.send(DispatchRequest {
            response_tx,
            request: Request::new(request_id, body),
            span: tracing::Span::current(),
        });

        // RequestGuard impls Drop to cancel in-flight requests. It should be created before
        // sending out the request; otherwise, the response future could be dropped after the
        // request is sent out but before ResponseGuard is created, rendering the cancellation
        // logic inactive.
        let guard: RequestGuard<Tsk, TskId> = RequestGuard {
            response_rx: &mut response_rx,
            to_dispatch_cancellation: &self.to_dispatch_cancellation,
            request_id,
            cancel: true,
        };

        guard.response().await
    }
}

/// A server response that is completed by request dispatch when the corresponding response
/// arrives off the wire.
struct RequestGuard<'a, Tsk: Task, TskId> {
    response_rx: &'a mut oneshot::Receiver<Result<Response<Tsk::Output, TskId>, DeadlineExceededError>>,
    to_dispatch_cancellation: &'a UnboundedSender<u64>,
    request_id: u64,
    cancel: bool,
}

/// An error that can occur in the processing of an RPC. This is not request-specific errors but
/// rather cross-cutting errors that can always occur.
#[derive(thiserror::Error, Clone, Debug, PartialEq, Eq, Hash)]
pub enum Error {
    /// The client disconnected from the consumer.
    #[error("the client disconnected from the consumer")]
    Disconnected,
    /// The request exceeded its deadline.
    #[error("the request exceeded its deadline")]
    DeadlineExceeded,
    /// The consumer aborted request processing.
    #[error("the consumer aborted request processing")]
    Consumer(#[from] ConsumerError),
}

impl<Tsk: Task, TskId> RequestGuard<'_, Tsk, TskId> {
    async fn response(mut self) -> Result<ResponseBody<Tsk::Output, TskId>, Error> {
        let response = (&mut self.response_rx).await;
        self.cancel = false;
        match response {
            Ok(Ok(response)) => Ok(response.result.map_err(Error::Consumer)?),
            Ok(Err(DeadlineExceededError)) => Err(Error::DeadlineExceeded),
            Err(oneshot::error::RecvError { .. }) => {
                // The oneshot is Canceled when the dispatch task ends. In that case,
                // there's nothing listening on the other side, so there's no point in
                // propagating cancellation.
                Err(Error::Disconnected)
            }
        }
    }
}

// Cancels the request when dropped, if not already complete.
impl<Tsk: Task, TskId> Drop for RequestGuard<'_, Tsk, TskId> {
    fn drop(&mut self) {
        // The receiver needs to be closed to handle the edge case that the request has not
        // yet been received by the dispatch task. It is possible for the cancel message to
        // arrive before the request itself, in which case the request could get stuck in the
        // dispatch map forever if the server never responds (e.g. if the server dies while
        // responding). Even if the server does respond, it will have unnecessarily done work
        // for a client no longer waiting for a response. To avoid this, the dispatch task
        // checks if the receiver is closed before inserting the request in the map. By
        // closing the receiver before sending the cancel message, it is guaranteed that if the
        // dispatch task misses an early-arriving cancellation message, then it will see the
        // receiver as closed.
        self.response_rx.close();
        if self.cancel {
            tracing::debug!(request_id = self.request_id, "to dispatch cancel request");
            let _ = self.to_dispatch_cancellation.send(self.request_id);
        }
    }
}
