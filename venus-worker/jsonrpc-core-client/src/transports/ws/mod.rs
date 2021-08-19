use std::collections::{HashMap, VecDeque};
use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::Duration;

use async_std::{
    channel::TryRecvError,
    prelude::{Future, FutureExt},
};
use async_tungstenite::{
    async_std::{connect_async, ConnectStream},
    tungstenite::{error::Result as wsResult, http::Request, Message},
    WebSocketStream,
};
use jsonrpc_core::{
    futures::{SinkExt, StreamExt},
    Id,
};
use jsonrpc_pubsub::SubscriptionId;
use serde_json::Value;
use tracing::{error, warn};

use super::RequestBuilder;
use crate::{channel::Receiver, parse_response, req, RpcError, RpcMessage, RpcResult};

use super::{PendingRequest, Subscription};

struct Client(WebSocketStream<ConnectStream>);

impl Client {
    fn handle_stream(
        &mut self,
        cx: &mut Context<'_>,
        incoming: &mut VecDeque<(Id, RpcResult<Value>, Option<String>, Option<SubscriptionId>)>,
        outgoing: &mut VecDeque<Message>,
    ) -> Result<(), String> {
        loop {
            match self.0.poll_next_unpin(cx) {
                Poll::Ready(Some(Ok(msg))) => match msg {
                    Message::Text(data) => {
                        match parse_response(&data) {
                            Ok(parsed) => incoming.push_back(parsed),
                            Err(e) => {
                                warn!(
                                    data = data.as_str(),
                                    "parse response from server side: {:?}", e
                                );
                            }
                        };
                    }

                    Message::Binary(data) => {
                        warn!("server sent bianry data, {} bytes", data.len());
                    }

                    m @ Message::Ping(_) => outgoing.push_front(m),
                    m @ Message::Close(_) => outgoing.push_front(m),

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
        outgoing: &mut VecDeque<Message>,
    ) -> Result<bool, String> {
        loop {
            match self.0.poll_ready_unpin(cx) {
                Poll::Ready(Ok(_)) => {}
                Poll::Ready(Err(e)) => return Err(format!("sink poll_ready: {:?}", e)),
                Poll::Pending => break,
            };

            match outgoing.pop_front() {
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

pub struct RequestInfo {
    url: String,
    headers: HashMap<String, String>,
}

impl RequestInfo {
    fn connect(
        &self,
        delay: Duration,
    ) -> Pin<Box<dyn Future<Output = wsResult<WebSocketStream<ConnectStream>>>>> {
        let mut builder = Request::builder().uri(self.url.to_owned());
        for (k, v) in self.headers.iter() {
            builder = builder.header(k, v);
        }

        let conn_fn = async move {
            let req = builder.body(())?;

            let (ws, _) = connect_async(req).delay(delay).await?;

            Ok(ws)
        };

        Box::pin(conn_fn)
    }
}

struct WS {
    done: crossbeam_channel::Receiver<()>,

    req_rx: Receiver<RpcMessage>,
    req_builder: RequestBuilder,

    pending_reqs: HashMap<Id, PendingRequest>,
    subs: HashMap<(SubscriptionId, String), Subscription>,

    incoming: VecDeque<(Id, RpcResult<Value>, Option<String>, Option<SubscriptionId>)>,
    outgoing: VecDeque<Message>,

    connect_req: RequestInfo,
    client: Option<Client>,
    reconnect: Option<Pin<Box<dyn Future<Output = wsResult<WebSocketStream<ConnectStream>>>>>>,
}

impl Future for WS {
    type Output = RpcResult<()>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let this = Pin::into_inner(self);

        // handle client reconnecting
        if let Some(fut) = this.reconnect.as_mut() {
            match Pin::new(fut).poll(cx) {
                Poll::Ready(Ok(ws)) => {
                    this.client.replace(Client(ws));
                    this.reconnect.take().map(drop);
                }

                Poll::Ready(Err(e)) => {
                    error!("connect error: {:?}", e);
                    this.try_reconnect();
                }

                Poll::Pending => {
                    return Poll::Pending;
                }
            }
        };

        // receive from requset rx
        // this will fill in self.outgoing
        if let Err(e) = this.handle_reqs() {
            return Poll::Ready(Err(e));
        }

        match this.handle_ws(cx) {
            Ok(_) => {}
            Err(reason) => {
                error!("ws client：{}", reason);
                this.cleanup(true);
                this.try_reconnect();
                return Poll::Pending;
            }
        };

        if !this.closed() {
            return Poll::Pending;
        }

        this.cleanup(false);
        Poll::Ready(Ok(()))
    }
}

impl Drop for WS {
    fn drop(&mut self) {
        self.cleanup(false);
    }
}

impl WS {
    fn closed(&self) -> bool {
        self.done.try_recv() != Err(crossbeam_channel::TryRecvError::Empty)
            || self.req_rx.is_closed()
    }

    fn cleanup(&mut self, is_temp: bool) {
        // TODO handle cleanup result

        for (_, req) in self.pending_reqs.drain() {
            match req {
                PendingRequest::Call(tx) => {
                    let _ = tx.send(Err(RpcError::ClientUnavailable(is_temp)));
                }

                PendingRequest::Subscription(sub) => {
                    let _ = sub.channel.send(Err(RpcError::ClientUnavailable(is_temp)));
                }
            }
        }

        for (_, sub) in self.subs.drain() {
            let _ = sub.channel.send(Err(RpcError::ClientUnavailable(is_temp)));
        }
    }

    fn try_reconnect(&mut self) {
        self.reconnect
            // TODO: configurable
            .replace(self.connect_req.connect(Duration::from_secs(30)))
            .map(drop);
    }

    fn handle_ws(&mut self, cx: &mut Context<'_>) -> Result<(), String> {
        // fill in self.incoming
        match self.client.as_mut() {
            Some(cli) => {
                cli.handle_stream(cx, &mut self.incoming, &mut self.outgoing)?;
            }
            None => return Ok(()),
        };

        // consume self.incoming
        self.handle_resps();

        // consume self.outgoing
        match self.client.as_mut() {
            Some(cli) => cli.handle_sink(cx, &mut self.outgoing).map(|_| ()),
            None => Ok(()),
        }
    }

    fn handle_reqs(&mut self) -> RpcResult<()> {
        loop {
            let msg = match self.req_rx.try_recv() {
                Ok(msg) => msg,

                Err(TryRecvError::Empty) => return Ok(()),

                Err(TryRecvError::Closed) => {
                    return Err(RpcError::Client("rpc request channel closed".to_owned()))
                }
            };

            let req_str = match msg {
                RpcMessage::Call(call) => {
                    let (id, req_str) = self.req_builder.call_request(&call);
                    self.pending_reqs
                        .insert(id, PendingRequest::Call(call.sender));
                    req_str
                }

                RpcMessage::Subscribe(sub) => {
                    let req::Subscription {
                        subscribe,
                        subscribe_params,
                        notification,
                        unsubscribe,
                    } = sub.subscription;

                    let (id, req_str) = self
                        .req_builder
                        .subscribe_request(subscribe, subscribe_params);

                    self.pending_reqs.insert(
                        id,
                        PendingRequest::Subscription(Subscription::new(
                            sub.sender,
                            notification,
                            unsubscribe,
                        )),
                    );

                    req_str
                }

                RpcMessage::Notify(notify) => self.req_builder.notification(&notify),
            };

            self.outgoing.push_back(Message::Text(req_str));
        }
    }

    fn handle_resps(&mut self) {
        while let Some((id, result, method, sid)) = self.incoming.pop_front() {
            match self.pending_reqs.remove(&id) {
                Some(pending) => {
                    match pending {
                        PendingRequest::Call(resp_tx) => {
                            if let Err(e) = resp_tx.send(result) {
                                error!("result tx for request #{:?} is unavailabe: {:?}", id, e);
                            }
                        }

                        PendingRequest::Subscription(mut sub) => {
                            let sub_chan_id = result
                                .as_ref()
                                .ok()
                                .and_then(|r| SubscriptionId::parse_value(r));

                            let method = sub.notification.clone();

                            match sub_chan_id {
                                Some(chan_id) => {
                                    sub.id = Some(chan_id.clone());
                                    self.subs.insert((chan_id, method), sub);
                                }

                                None => {
                                    if let Err(e) = sub.channel.try_send(result) {
                                        error!(
                                        "subscription tx for request #{:?} is unavailable: {:?}",
                                        id, e
                                    );
                                    }
                                }
                            };
                        }
                    }
                    continue;
                }

                None => {
                    if sid.is_none() || method.is_none() {
                        warn!(
                            "unexpected response with id {:?} ({:?}, {:?})",
                            id, sid, method,
                        );
                        continue;
                    }
                }
            }

            // seems we got an notification
            let sub_ident = (sid.unwrap(), method.unwrap());
            if let Some(sub) = self.subs.get_mut(&sub_ident) {
                if let Err(e) = sub.channel.try_send(result) {
                    error!(
                        "subscription tx for notification {:?} is not available: {:?}",
                        sub_ident, e
                    );

                    let removed = self.subs.remove(&sub_ident).unwrap();

                    let (_id, req_str) = self
                        .req_builder
                        .unsubscribe_request(removed.unsubscribe, sub_ident.0);

                    self.outgoing.push_back(Message::Text(req_str));
                }
            }
        }
    }
}
