use std::collections::VecDeque;
use std::future::Future;
use std::pin::Pin;
use std::task::Context;
use std::time::Duration;

use jsonrpc_core::{Call, Id, MethodCall, Notification, Params, Request, Version};
use jsonrpc_pubsub::SubscriptionId;
use serde_json::Value;

use crate::channel::{oneshot, Sender};
use crate::{CallMessage, NotifyMessage, RpcResult};

pub mod duplex;
pub mod local;
pub mod ws;

pub trait Client: Sized + Unpin {
    type ConnectInfo: Unpin;
    type ConnectError: std::error::Error;

    fn connect(
        info: &Self::ConnectInfo,
        dealy: Option<Duration>,
    ) -> Pin<Box<dyn Future<Output = Result<Self, Self::ConnectError>> + Send>>;

    fn handle_stream(
        &mut self,
        cx: &mut Context<'_>,
        incoming: &mut VecDeque<String>,
    ) -> Result<(), String>;

    fn handle_sink(
        &mut self,
        cx: &mut Context<'_>,
        outgoing: &mut VecDeque<String>,
    ) -> Result<bool, String>;
}

struct Subscription {
    /// Subscription id received when subscribing.
    id: Option<SubscriptionId>,
    /// A method name used for notification.
    notification: String,
    /// Rpc method to unsubscribe.
    unsubscribe: String,
    /// Where to send messages to.
    channel: Sender<RpcResult<Value>>,
}

impl Subscription {
    fn new(channel: Sender<RpcResult<Value>>, notification: String, unsubscribe: String) -> Self {
        Subscription {
            id: None,
            notification,
            unsubscribe,
            channel,
        }
    }
}

enum PendingRequest {
    Call(oneshot::Tx<RpcResult<Value>>),
    Subscription(Subscription),
}

pub struct RequestBuilder {
    id: u64,
}

impl RequestBuilder {
    /// Create a new RequestBuilder
    pub fn new() -> Self {
        RequestBuilder { id: 0 }
    }

    fn next_id(&mut self) -> Id {
        let id = self.id;
        self.id = id + 1;
        Id::Num(id)
    }

    /// Build a single request with the next available id
    fn single_request(&mut self, method: String, params: Params) -> (Id, String) {
        let id = self.next_id();
        let request = Request::Single(Call::MethodCall(MethodCall {
            jsonrpc: Some(Version::V2),
            method,
            params,
            id: id.clone(),
        }));
        (
            id,
            serde_json::to_string(&request).expect("Request serialization is infallible; qed"),
        )
    }

    fn call_request(&mut self, msg: &CallMessage) -> (Id, String) {
        self.single_request(msg.method.clone(), msg.params.clone())
    }

    fn subscribe_request(&mut self, subscribe: String, subscribe_params: Params) -> (Id, String) {
        self.single_request(subscribe, subscribe_params)
    }

    pub fn unsubscribe_request(
        &mut self,
        unsubscribe: String,
        sid: SubscriptionId,
    ) -> (Id, String) {
        self.single_request(unsubscribe, Params::Array(vec![Value::from(sid)]))
    }

    fn notification(&mut self, msg: &NotifyMessage) -> String {
        let request = jsonrpc_core::Request::Single(Call::Notification(Notification {
            jsonrpc: Some(Version::V2),
            method: msg.method.clone(),
            params: msg.params.clone(),
        }));
        serde_json::to_string(&request).expect("Request serialization is infallible; qed")
    }
}
