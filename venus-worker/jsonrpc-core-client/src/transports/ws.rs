use std::collections::{HashMap, VecDeque};
use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::Duration;

use async_std::prelude::FutureExt;
use async_tungstenite::{
    async_std::{connect_async, ConnectStream},
    tungstenite::{error::Error as wsError, http::Request, Message},
    WebSocketStream,
};
use jsonrpc_core::futures::{SinkExt, StreamExt};
use tracing::warn;

use super::Client;

pub struct ConnectInfo {
    pub url: String,
    pub headers: HashMap<String, String>,
}

pub struct WS(WebSocketStream<ConnectStream>, VecDeque<Message>);

impl Client for WS {
    type ConnectInfo = ConnectInfo;
    type ConnectError = wsError;

    fn connect(
        info: &Self::ConnectInfo,
        dealy: Option<Duration>,
    ) -> Pin<Box<dyn Future<Output = Result<Self, Self::ConnectError>>>> {
        let mut builder = Request::builder().uri(info.url.to_owned());
        for (k, v) in info.headers.iter() {
            builder = builder.header(k, v);
        }

        let conn_fn = async move {
            let req = builder.body(())?;

            let (ws, _) = connect_async(req).await?;

            Ok(WS(ws, VecDeque::new()))
        };

        match dealy {
            Some(d) => Box::pin(conn_fn.delay(d)),
            None => Box::pin(conn_fn),
        }
    }

    fn handle_stream(
        &mut self,
        cx: &mut Context<'_>,
        incoming: &mut VecDeque<String>,
    ) -> Result<(), String> {
        loop {
            match self.0.poll_next_unpin(cx) {
                Poll::Ready(Some(Ok(msg))) => match msg {
                    Message::Text(data) => {
                        incoming.push_back(data);
                    }

                    Message::Binary(data) => {
                        warn!("server sent bianry data, {} bytes", data.len());
                    }

                    m @ Message::Ping(_) => self.1.push_front(m),
                    m @ Message::Close(_) => self.1.push_front(m),

                    Message::Pong(_) => {}
                },

                Poll::Ready(None) => {
                    return Err("connection closed".to_owned());
                }

                Poll::Ready(Some(Err(e))) => {
                    return Err(format!("stream poll_next: {:?}", e));
                }

                Poll::Pending => return Ok(()),
            }
        }
    }

    fn handle_sink(
        &mut self,
        cx: &mut Context<'_>,
        outgoing: &mut VecDeque<String>,
    ) -> Result<bool, String> {
        while let Some(msg) = outgoing.pop_front() {
            self.1.push_back(Message::Text(msg));
        }

        loop {
            match self.0.poll_ready_unpin(cx) {
                Poll::Ready(Ok(_)) => {}
                Poll::Ready(Err(e)) => return Err(format!("sink poll_ready: {:?}", e)),
                Poll::Pending => break,
            };

            match self.1.pop_front() {
                Some(msg) => {
                    if let Err(e) = self.0.start_send_unpin(msg) {
                        return Err(format!("sink start_send: {:?}", e));
                    }
                }

                None => break,
            }
        }

        match self.0.poll_flush_unpin(cx) {
            Poll::Ready(Ok(_)) => Ok(true),
            Poll::Ready(Err(e)) => Err(format!("sink poll_flush: {:?}", e)),
            Poll::Pending => Ok(false),
        }
    }
}
