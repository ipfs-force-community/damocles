use std::{
    future::Future,
    marker::PhantomData,
    pin::Pin,
    task::{ready, Context, Poll},
};

use futures_util::{stream::Fuse, Sink, SinkExt, Stream, StreamExt, TryStreamExt};
use tokio::sync::mpsc::{unbounded_channel, UnboundedReceiver, UnboundedSender};
use tracing::Instrument;

use crate::{context::SpanExt, response::ConsumerError, Consumer, Request, Response, Task};

use super::routing::Routing;

/// Handles the lifecycle of requests, read requests from the transport,
/// and writing responses to the transport.
pub struct Dispatch<Tsk: Task, C: Consumer<Tsk>, TP> {
    consumer: C,
    transport: Fuse<TP>,
    to_dispatch_response: UnboundedSender<Response<Tsk::Output, C::TaskId>>,
    pending_responses: UnboundedReceiver<Response<Tsk::Output, C::TaskId>>,
    response_buffered: Option<Response<Tsk::Output, C::TaskId>>,
    _pd: PhantomData<Tsk>,
}

impl<Tsk: Task, C: Consumer<Tsk>, TP: Stream> Dispatch<Tsk, C, TP> {
    pub fn new(consumer: C, transport: TP) -> Self {
        let (to_dispatch_response, pending_responses) = unbounded_channel();
        Self {
            consumer,
            transport: transport.fuse(),
            to_dispatch_response,
            pending_responses,
            response_buffered: None,
            _pd: PhantomData,
        }
    }
}

impl<Tsk, C, TP, E> Dispatch<Tsk, C, TP>
where
    Tsk: Task,
    C: Consumer<Tsk> + Clone + Send + 'static,
    C::TaskId: Send,
    C::Error: Into<ConsumerError>,
    C::StartTaskFut: Send,
    C::WaitTaskFut: Send,
    TP: Sink<Response<Tsk::Output, C::TaskId>, Error = E> + Stream<Item = Result<Request<Tsk, C::TaskId>, E>> + Unpin,
{
    fn poll_read_request(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        while let Some(request) = ready!(self.transport.try_poll_next_unpin(cx)?) {
            let span = tracing::Span::current();
            span.set_context(&request.context);
            tracing::debug!(request_id = request.request_id, "polled new request");
            let to_dispatch_response = self.to_dispatch_response.clone();
            let mut consumer = self.consumer.clone();
            tokio::spawn(async move {
                if let Some(response) = consumer.call(request).instrument(span).await {
                    let _ = to_dispatch_response.send(response);
                }
            });
        }
        Poll::Ready(Ok(()))
    }

    fn poll_write_response(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        if let Some(response) = self.response_buffered.take() {
            ready!(self.poll_start_send_response(cx, response))?;
        }
        loop {
            match self.pending_responses.poll_recv(cx) {
                Poll::Ready(Some(response)) => ready!(self.poll_start_send_response(cx, response))?,
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

    fn poll_start_send_response(
        &mut self,
        cx: &mut Context<'_>,
        response: Response<Tsk::Output, C::TaskId>,
    ) -> Poll<Result<(), TP::Error>> {
        debug_assert!(self.response_buffered.is_none());
        match self.transport.poll_ready_unpin(cx)? {
            Poll::Ready(()) => Poll::Ready(self.transport.start_send_unpin(response)),
            Poll::Pending => {
                self.response_buffered = Some(response);
                Poll::Pending
            }
        }
    }
}

// Pinning is never projected to any fields
impl<Tsk: Task, C: Consumer<Tsk>, TP> Unpin for Dispatch<Tsk, C, TP> where TP: Unpin {}

impl<Tsk, C, TP, E> Future for Dispatch<Tsk, C, TP>
where
    Tsk: Task,
    C: Consumer<Tsk> + Clone + Send + 'static,
    C::TaskId: Send,
    C::Error: Into<ConsumerError>,
    C::StartTaskFut: Send,
    C::WaitTaskFut: Send,
    TP: Sink<Response<Tsk::Output, C::TaskId>, Error = E> + Stream<Item = Result<Request<Tsk, C::TaskId>, E>> + Unpin,
{
    type Output = Result<(), E>;

    fn poll(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        match (self.poll_read_request(cx)?, self.poll_write_response(cx)?) {
            (Poll::Ready(()), Poll::Ready(())) => Poll::Ready(Ok(())),
            _ => Poll::Pending,
        }
    }
}
