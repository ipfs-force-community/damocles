//! utilities jsonrpc client based on websocket

use std::collections::VecDeque;
use std::pin::Pin;
use std::task::{Context, Poll};

use anyhow::{anyhow, Result};
use async_std::task::spawn;
use async_tungstenite::{async_std::connect_async, tungstenite::Message};
use jsonrpc_core_client::{
    futures::{future::ready, Sink, SinkExt, Stream, StreamExt},
    transports::duplex,
    RpcChannel, RpcError,
};

pub use async_tungstenite::tungstenite::http::Request;

use super::LOG_TARGET;
use crate::logging::{debug_span, warn, Span};

/// try to connect to the given target
pub async fn connect<T>(req: Request<()>) -> Result<T>
where
    T: From<RpcChannel>,
{
    let (ws_stream, _) = connect_async(req).await?;
    let (ws_sink, ws_stream) = ws_stream.split();

    let span = debug_span!(target: LOG_TARGET, "ws");
    let ws_client = WSClient {
        sink: ws_sink.sink_map_err(|e| RpcError::Other(Box::new(e))),
        stream: ws_stream.map(|res| res.map_err(|e| RpcError::Other(Box::new(e)))),
        queue: VecDeque::new(),
        span: span,
    };

    let (sink, stream) = ws_client.split();
    let sink = Box::pin(sink);
    let stream = Box::pin(
        stream
            .take_while(|res| ready(res.is_ok()))
            .map(|res| res.expect("stream should be closed upon first error")),
    );

    let (rpc_client, sender) = duplex(sink, stream);
    spawn(rpc_client)
        .await
        .map_err(|e| anyhow!("spawn duplex: {}", e))?;

    Ok(sender.into())
}

struct WSClient<TSink, TStream> {
    sink: TSink,
    stream: TStream,
    queue: VecDeque<Message>,
    span: Span,
}

impl<TSink, TStream> WSClient<TSink, TStream>
where
    TSink: Sink<Message, Error = RpcError> + Unpin,
    TStream: Stream<Item = Result<Message, RpcError>> + Unpin,
{
    // Drains the internal buffer and attempts to forward as much of the items
    // as possible to the underlying sink
    fn try_empty_buffer(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<Result<(), TSink::Error>> {
        let this = Pin::into_inner(self);

        match Pin::new(&mut this.sink).poll_ready(cx) {
            Poll::Ready(value) => value?,
            Poll::Pending => return Poll::Pending,
        }

        while let Some(item) = this.queue.pop_front() {
            Pin::new(&mut this.sink).start_send(item)?;

            if !this.queue.is_empty() {
                match Pin::new(&mut this.sink).poll_ready(cx) {
                    Poll::Ready(value) => value?,
                    Poll::Pending => return Poll::Pending,
                }
            }
        }

        Poll::Ready(Ok(()))
    }
}

impl<TSink, TStream> Sink<String> for WSClient<TSink, TStream>
where
    TSink: Sink<Message, Error = RpcError> + Unpin,
    TStream: Stream<Item = Result<Message, RpcError>> + Unpin,
{
    type Error = RpcError;

    fn poll_ready(self: Pin<&mut Self>, ctx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        let this = Pin::into_inner(self);

        if this.queue.is_empty() {
            return Pin::new(&mut this.sink).poll_ready(ctx);
        }

        let _ = Pin::new(this).try_empty_buffer(ctx)?;

        Poll::Ready(Ok(()))
    }

    fn start_send(mut self: Pin<&mut Self>, item: String) -> Result<(), Self::Error> {
        let request = Message::Text(item);

        if self.queue.is_empty() {
            let this = Pin::into_inner(self);
            Pin::new(&mut this.sink).start_send(request)
        } else {
            self.queue.push_back(request);
            Ok(())
        }
    }

    fn poll_flush(self: Pin<&mut Self>, ctx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        let this = Pin::into_inner(self);

        match Pin::new(&mut *this).try_empty_buffer(ctx) {
            Poll::Ready(value) => value?,
            Poll::Pending => return Poll::Pending,
        }

        Pin::new(&mut this.sink).poll_flush(ctx)
    }

    fn poll_close(self: Pin<&mut Self>, ctx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        let this = Pin::into_inner(self);

        match Pin::new(&mut *this).try_empty_buffer(ctx) {
            Poll::Ready(value) => value?,
            Poll::Pending => return Poll::Pending,
        }

        Pin::new(&mut this.sink).poll_close(ctx)
    }
}

impl<TSink, TStream> Stream for WSClient<TSink, TStream>
where
    TSink: Sink<Message, Error = RpcError> + Unpin,
    TStream: Stream<Item = Result<Message, RpcError>> + Unpin,
{
    type Item = Result<String, RpcError>;

    fn poll_next(self: Pin<&mut Self>, ctx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        let this = Pin::into_inner(self);
        loop {
            match Pin::new(&mut this.stream).poll_next(ctx) {
                Poll::Ready(Some(Ok(message))) => match message {
                    Message::Text(data) => return Poll::Ready(Some(Ok(data))),
                    Message::Binary(data) => {
                        warn!(parent: &this.span, "server sent binary data {:?}", data);
                    }

                    m @ Message::Ping(_) => this.queue.push_front(m),
                    m @ Message::Close(_) => this.queue.push_front(m),

                    Message::Pong(_) => {}
                },
                Poll::Ready(None) => {
                    // TODO try to reconnect (#411).
                    return Poll::Ready(None);
                }
                Poll::Pending => return Poll::Pending,
                Poll::Ready(Some(Err(error))) => {
                    return Poll::Ready(Some(Err(RpcError::Other(Box::new(error)))))
                }
            }
        }
    }
}
