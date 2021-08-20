use std::collections::{HashMap, VecDeque};
use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::Duration;

use async_std::{channel::TryRecvError, prelude::Future};
use jsonrpc_core::Id;
use jsonrpc_pubsub::SubscriptionId;
use tracing::{error, warn};

use super::{Client, PendingRequest, RequestBuilder, Subscription};
use crate::{
    channel::{unbounded, Receiver},
    parse_response, req, RpcChannel, RpcError, RpcMessage, RpcResult,
};

pub struct Duplex<C: Client> {
    done: crossbeam_channel::Receiver<()>,

    req_rx: Receiver<RpcMessage>,
    req_builder: RequestBuilder,

    pending_reqs: HashMap<Id, PendingRequest>,
    subs: HashMap<(SubscriptionId, String), Subscription>,

    incoming: VecDeque<String>,
    outgoing: VecDeque<String>,

    connect_info: C::ConnectInfo,
    client: Option<C>,
    reconnect: Option<Pin<Box<dyn Future<Output = Result<C, C::ConnectError>> + Send>>>,
}

impl<C: Client> Duplex<C> {
    pub async fn new(
        done: crossbeam_channel::Receiver<()>,
        info: C::ConnectInfo,
    ) -> RpcResult<(Self, RpcChannel)> {
        let client = C::connect(&info, None)
            .await
            .map_err(|e| RpcError::Client(format!("connect failed: {:?}", e)))?;

        let (req_tx, req_rx) = unbounded();

        Ok((
            Duplex {
                done,
                req_rx,
                req_builder: RequestBuilder::new(),
                pending_reqs: HashMap::new(),
                subs: HashMap::new(),
                incoming: VecDeque::new(),
                outgoing: VecDeque::new(),
                connect_info: info,
                client: Some(client),
                reconnect: None,
            },
            req_tx.into(),
        ))
    }
}

impl<C: Client> Future for Duplex<C> {
    type Output = RpcResult<()>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let this = Pin::into_inner(self);

        // handle client reconnecting
        if let Some(fut) = this.reconnect.as_mut() {
            match Pin::new(fut).poll(cx) {
                Poll::Ready(Ok(cli)) => {
                    this.client.replace(cli);
                    this.reconnect.take().map(drop);
                }

                Poll::Ready(Err(e)) => {
                    error!("reconnect failed: {:?}", e);
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

        match this.handle_client(cx) {
            Ok(_) => {}
            Err(reason) => {
                error!("client：{}", reason);
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

impl<C: Client> Drop for Duplex<C> {
    fn drop(&mut self) {
        self.cleanup(false);
    }
}

impl<C: Client> Duplex<C> {
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
            .replace(C::connect(
                &self.connect_info,
                Some(Duration::from_secs(30)),
            ))
            .map(drop);
    }

    fn handle_client(&mut self, cx: &mut Context<'_>) -> Result<(), String> {
        // fill in self.incoming
        match self.client.as_mut() {
            Some(cli) => {
                cli.handle_stream(cx, &mut self.incoming)?;
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

            self.outgoing.push_back(req_str);
        }
    }

    fn handle_resps(&mut self) {
        while let Some(resp_str) = self.incoming.pop_front() {
            let (id, result, method, sid) = match parse_response(&resp_str) {
                Ok(parsed) => parsed,
                Err(e) => {
                    warn!(
                        content = resp_str.as_str(),
                        "parse response from server side: {:?}", e
                    );
                    continue;
                }
            };

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

                    self.outgoing.push_back(req_str);
                }
            }
        }
    }
}
