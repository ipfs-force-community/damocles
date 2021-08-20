use async_std::{channel::unbounded, future::Future};
use jsonrpc_core::Params;
use serde::{de::DeserializeOwned, Serialize};
use serde_json::Value;
use tracing::debug;

use crate::{
    channel::oneshot, CallMessage, NotifyMessage, RpcChannel, RpcError, RpcResult,
    SubscribeMessage, Subscription, SubscriptionStream, TypedSubscriptionStream,
};

/// Client for raw JSON RPC requests
#[derive(Clone)]
pub struct RawClient(RpcChannel);

impl From<RpcChannel> for RawClient {
    fn from(channel: RpcChannel) -> Self {
        RawClient(channel)
    }
}

impl RawClient {
    /// Call RPC method with raw JSON.
    pub fn call_method(
        &self,
        method: &str,
        params: Params,
    ) -> impl Future<Output = RpcResult<Value>> {
        let (sender, receiver) = oneshot::oneshot();
        let msg = CallMessage {
            method: method.into(),
            params,
            sender,
        };

        let result = self.0.send(msg.into());
        async move {
            let () = result.map_err(|e| {
                let is_temp = !e.is_closed();
                RpcError::ClientUnavailable(is_temp)
            })?;

            receiver
                .await
                .map_err(|_| RpcError::ClientUnavailable(true))?
        }
    }

    /// Send RPC notification with raw JSON.
    pub fn notify(&self, method: &str, params: Params) -> RpcResult<()> {
        let msg = NotifyMessage {
            method: method.into(),
            params,
        };
        match self.0.send(msg.into()) {
            Ok(()) => Ok(()),
            Err(error) => Err(RpcError::Other(Box::new(error))),
        }
    }

    /// Subscribe to topic with raw JSON.
    pub fn subscribe(
        &self,
        subscribe: &str,
        subscribe_params: Params,
        notification: &str,
        unsubscribe: &str,
    ) -> RpcResult<SubscriptionStream> {
        let (sender, receiver) = unbounded();
        let msg = SubscribeMessage {
            subscription: Subscription {
                subscribe: subscribe.into(),
                subscribe_params,
                notification: notification.into(),
                unsubscribe: unsubscribe.into(),
            },
            sender,
        };

        self.0
            .send(msg.into())
            .map(|()| receiver)
            .map_err(|e| RpcError::Other(Box::new(e)))
    }
}

/// Client for typed JSON RPC requests
#[derive(Clone)]
pub struct TypedClient(RawClient);

impl From<RpcChannel> for TypedClient {
    fn from(channel: RpcChannel) -> Self {
        TypedClient(channel.into())
    }
}

impl TypedClient {
    /// Create a new `TypedClient`.
    pub fn new(raw_cli: RawClient) -> Self {
        TypedClient(raw_cli)
    }

    /// Call RPC with serialization of request and deserialization of response.
    pub fn call_method<T: Serialize, R: DeserializeOwned>(
        &self,
        method: &str,
        returns: &str,
        args: T,
    ) -> impl Future<Output = RpcResult<R>> {
        let returns = returns.to_owned();
        let args = serde_json::to_value(args)
            .expect("Only types with infallible serialisation can be used for JSON-RPC");
        let params = match args {
            Value::Array(vec) => Ok(Params::Array(vec)),
            Value::Null => Ok(Params::None),
            Value::Object(map) => Ok(Params::Map(map)),
            _ => Err(RpcError::Client(
                "RPC params should serialize to a JSON array, JSON object or null".into(),
            )),
        };
        let result = params.map(|params| self.0.call_method(method, params));

        async move {
            let value: Value = result?.await?;

            debug!("response: {:?}", value);

            serde_json::from_value::<R>(value)
                .map_err(|error| RpcError::ParseError(returns, Box::new(error)))
        }
    }

    /// Call RPC with serialization of request only.
    pub fn notify<T: Serialize>(&self, method: &str, args: T) -> RpcResult<()> {
        let args = serde_json::to_value(args)
            .expect("Only types with infallible serialisation can be used for JSON-RPC");
        let params = match args {
            Value::Array(vec) => Params::Array(vec),
            Value::Null => Params::None,
            _ => {
                return Err(RpcError::Client(
                    "RPC params should serialize to a JSON array, or null".into(),
                ))
            }
        };

        self.0.notify(method, params)
    }

    /// Subscribe with serialization of request and deserialization of response.
    pub fn subscribe<T: Serialize, R: DeserializeOwned + 'static>(
        &self,
        subscribe: &str,
        subscribe_params: T,
        topic: &str,
        unsubscribe: &str,
        returns: &'static str,
    ) -> RpcResult<TypedSubscriptionStream<R>> {
        let args = serde_json::to_value(subscribe_params)
            .expect("Only types with infallible serialisation can be used for JSON-RPC");

        let params = match args {
            Value::Array(vec) => Params::Array(vec),
            Value::Null => Params::None,
            _ => {
                return Err(RpcError::Client(
                    "RPC params should serialize to a JSON array, or null".into(),
                ))
            }
        };

        self.0
            .subscribe(subscribe, params, topic, unsubscribe)
            .map(move |stream| TypedSubscriptionStream::new(stream, returns))
    }
}
