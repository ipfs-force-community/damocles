use std::{
    fmt,
    future::Future,
    pin::Pin,
    task::{ready, Context, Poll},
};

use futures_util::{stream::Fuse, Sink, SinkExt, Stream, StreamExt, TryStreamExt};
use tokio::sync::{mpsc::UnboundedReceiver, oneshot};

use crate::{Request, Response, Task};

use super::inflight_requests::{DeadlineExceededError, InflightRequests};

pub struct DispatchRequest<Tsk: Task, TskId> {
    pub(crate) response_tx: oneshot::Sender<Result<Response<Tsk::Output, TskId>, DeadlineExceededError>>,
    pub(crate) request: Request<Tsk, TskId>,
    pub(crate) span: tracing::Span,
}

/// Handles the lifecycle of requests, writing requests to the transport, managing cancellations,
/// and reading responses from the transport.
#[derive(Debug)]
pub struct Dispatch<TP, Tsk: Task, TskId> {
    transport: Fuse<TP>,
    inflight_requests: InflightRequests<Tsk, TskId>,
    pending_cancellations: UnboundedReceiver<u64>,
    pending_requests: UnboundedReceiver<DispatchRequest<Tsk, TskId>>,
    request_buffered: Option<Request<Tsk, TskId>>,
}

impl<TP, Tsk, TskId, E> Dispatch<TP, Tsk, TskId>
where
    Tsk: Task,
    TP: Sink<Request<Tsk, TskId>, Error = E> + Stream<Item = Result<Response<Tsk::Output, TskId>, E>> + Unpin,
{
    pub fn new(
        transport: TP,
        pending_cancellations: UnboundedReceiver<u64>,
        pending_requests: UnboundedReceiver<DispatchRequest<Tsk, TskId>>,
    ) -> Self {
        Self {
            transport: transport.fuse(),
            inflight_requests: InflightRequests::new(),
            pending_cancellations,
            pending_requests,
            request_buffered: None,
        }
    }

    fn poll_write_cancellation(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        // If we've got an request buffered already, we need to write it to the
        // transport before we can do anything else
        if let Some(req) = self.request_buffered.take() {
            ready!(self.poll_start_send_request(cx, req))?;
        }
        loop {
            match self.pending_cancellations.poll_recv(cx) {
                Poll::Ready(Some(request_id)) => {
                    if let Some(span) = self.inflight_requests.cancel(request_id) {
                        ready!(self.poll_start_send_request(cx, Request::new_cancel_request(request_id, span)))?
                    }
                }
                Poll::Ready(None) => {
                    ready!(self.transport.poll_close_unpin(cx))?;
                    return Poll::Ready(Ok(()));
                }
                Poll::Pending => {
                    ready!(self.transport.poll_flush_unpin(cx))?;
                    return Poll::Pending;
                }
            }
        }
    }

    fn poll_expired_request(&mut self, cx: &mut Context<'_>) -> Poll<()> {
        while ready!(self.inflight_requests.poll_expired(cx)).is_some() {}
        Poll::Ready(())
    }

    fn poll_read_response(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        while let Some(response) = ready!(self.transport.try_poll_next_unpin(cx)?) {
            tracing::debug!(request_id = response.request_id, "polled new response");
            self.inflight_requests.complete(response);
        }
        Poll::Ready(Ok(()))
    }

    fn poll_write_request(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        // If we've got an request buffered already, we need to write it to the
        // transport before we can do anything else
        if let Some(req) = self.request_buffered.take() {
            ready!(self.poll_start_send_request(cx, req))?;
        }

        loop {
            match self.pending_requests.poll_recv(cx) {
                Poll::Ready(Some(dispatch_request)) => {
                    self.inflight_requests
                        .send(&dispatch_request.request, dispatch_request.response_tx, dispatch_request.span);
                    ready!(self.poll_start_send_request(cx, dispatch_request.request))?
                }
                Poll::Ready(None) => {
                    ready!(self.transport.poll_close_unpin(cx))?;
                    return Poll::Ready(Ok(()));
                }
                Poll::Pending => {
                    ready!(self.transport.poll_flush_unpin(cx))?;
                    return Poll::Pending;
                }
            }
        }
    }

    fn poll_start_send_request(&mut self, cx: &mut Context<'_>, req: Request<Tsk, TskId>) -> Poll<Result<(), TP::Error>> {
        debug_assert!(self.request_buffered.is_none());
        match self.transport.poll_ready_unpin(cx)? {
            Poll::Ready(()) => {
                tracing::debug!(request_id = req.request_id, "send request");
                Poll::Ready(self.transport.start_send_unpin(req))
            }
            Poll::Pending => {
                self.request_buffered = Some(req);
                Poll::Pending
            }
        }
    }

    fn inner_poll(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        match (
            self.poll_write_request(cx)?,
            self.poll_read_response(cx)?,
            self.poll_expired_request(cx),
            self.poll_write_cancellation(cx)?,
        ) {
            (Poll::Ready(()), Poll::Ready(()), Poll::Ready(()), Poll::Ready(())) => Poll::Ready(Ok(())),
            _ => Poll::Pending,
        }
    }
}

// Pinning is never projected to any fields
impl<TP, Tsk: Task, TskId> Unpin for Dispatch<TP, Tsk, TskId> where TP: Unpin {}

impl<TP, Tsk: Task, TskId, E> Future for Dispatch<TP, Tsk, TskId>
where
    TP: Sink<Request<Tsk, TskId>, Error = E> + Stream<Item = Result<Response<Tsk::Output, TskId>, E>> + Unpin,
    E: fmt::Debug,
{
    type Output = ();

    fn poll(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        loop {
            if let Err(e) = ready!(self.inner_poll(cx)) {
                tracing::error!(err=?e, "Connection broken");
            }
        }
    }
}
