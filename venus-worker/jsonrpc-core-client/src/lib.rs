use std::{
    marker::PhantomData,
    pin::Pin,
    task::{Context, Poll},
};

use async_std::{
    channel::{Receiver, Sender, TrySendError},
    stream::Stream,
    task::ready,
};
use jsonrpc_core::Error;
use serde::de::DeserializeOwned;
use serde_json::Value;

mod req;
use req::*;
mod resp;
use resp::*;

mod client;
pub use client::*;
mod channel;
pub use channel::*;

pub use jsonrpc_core::futures;

pub mod transports;
// pub mod duplex;
// pub mod local;
// pub mod ws;

type BoxedError = Box<dyn std::error::Error + Send>;

/// The errors returned by the client.
#[derive(Debug, derive_more::Display)]
pub enum RpcError {
    /// An error returned by the server.
    #[display(fmt = "Server returned rpc error {}", _0)]
    JsonRpcError(Error),
    /// Failure to parse server response.
    #[display(fmt = "Failed to parse server response as {}: {}", _0, _1)]
    ParseError(String, BoxedError),
    /// Request timed out.
    #[display(fmt = "Request timed out")]
    Timeout,

    /// A general client error.
    #[display(fmt = "Client error: {}", _0)]
    Client(String),
    /// Not rpc specific errors.
    #[display(fmt = "{}", _0)]
    Other(BoxedError),

    #[display(fmt = "underlying client is unavailable, temporary={}", _0)]
    ClientUnavailable(bool),
}

impl std::error::Error for RpcError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match *self {
            Self::JsonRpcError(ref e) => Some(e),
            Self::ParseError(_, ref e) => Some(&**e),
            Self::Other(ref e) => Some(&**e),
            _ => None,
        }
    }
}

impl From<Error> for RpcError {
    fn from(error: Error) -> Self {
        RpcError::JsonRpcError(error)
    }
}

pub type RpcResult<T> = Result<T, RpcError>;

#[derive(Clone)]
pub struct RpcChannel(Sender<RpcMessage>);

impl RpcChannel {
    fn send(&self, msg: RpcMessage) -> Result<(), TrySendError<RpcMessage>> {
        self.0.try_send(msg)
    }
}

impl From<Sender<RpcMessage>> for RpcChannel {
    fn from(sender: Sender<RpcMessage>) -> Self {
        RpcChannel(sender)
    }
}

pub type RpcFuture = Receiver<Result<Value, RpcError>>;

/// The stream returned by a subscribe.
pub type SubscriptionStream = Receiver<Result<Value, RpcError>>;

/// A typed subscription stream.
pub struct TypedSubscriptionStream<T> {
    _marker: PhantomData<T>,
    returns: &'static str,
    stream: SubscriptionStream,
}

impl<T> TypedSubscriptionStream<T> {
    /// Creates a new `TypedSubscriptionStream`.
    pub fn new(stream: SubscriptionStream, returns: &'static str) -> Self {
        TypedSubscriptionStream {
            _marker: PhantomData,
            returns,
            stream,
        }
    }
}

impl<T: DeserializeOwned + Unpin + 'static> Stream for TypedSubscriptionStream<T> {
    type Item = RpcResult<T>;

    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context) -> Poll<Option<Self::Item>> {
        let result = ready!(Pin::new(&mut self.stream).poll_next(cx));
        match result {
            Some(Ok(value)) => Some(
                serde_json::from_value::<T>(value)
                    .map_err(|error| RpcError::ParseError(self.returns.into(), Box::new(error))),
            ),
            None => None,
            Some(Err(err)) => Some(Err(err)),
        }
        .into()
    }
}
