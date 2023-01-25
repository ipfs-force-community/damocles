use std::{
    error::Error as StdError,
    fmt, io,
    ops::{Deref, DerefMut},
    pin::Pin,
    str::FromStr,
    task::{Context, Poll},
};

use framed::FramedExt;
use futures::{ready, Sink, Stream};
use pin_project::pin_project;
use readwriter::{
    reconnect::{self, dyn_retry_iter, DynReconnect, Reconnect},
    BoxReadWriter, ReadWriterExt,
};
use serde::{Deserialize, Serialize};
use serded::Serded;
use tokio::sync::mpsc;
use tokio_util::codec::Framed as TokioFramed;

pub mod client;
pub mod framed;
pub mod readwriter;
pub mod serded;
pub mod server;

/// Request contains the required data to be sent to the consumer.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Request<T> {
    /// request id which should be maintained by the producer and used later to dispatch the response
    pub id: u64,
    /// the task body
    pub body: T,
}

/// Response contains the output for the specific task, and error message if exists.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Response<T> {
    /// request id
    pub id: u64,

    /// the response body
    pub body: T,
}

#[derive(thiserror::Error, Debug)]
pub enum TransportError<E>
where
    E: StdError + 'static,
{
    /// Could not ready the transport for writes.
    #[error("could not ready the transport for writes")]
    Ready(#[source] E),
    /// Could not read from the transport.
    #[error("could not read from the transport")]
    Read(#[source] E),
    /// Could not write to the transport.
    #[error("could not write to the transport")]
    Write(#[source] E),
    /// Could not flush the transport.
    #[error("could not flush the transport")]
    Flush(#[source] E),
    /// Could not close the transport.
    #[error("could not close the transport")]
    Close(#[source] E),
}

pub trait Transport<SinkItem, Item>: Sink<SinkItem> + Stream<Item = Result<Item, Self::Error>> {}

impl<T, SinkItem, Item, E> Transport<SinkItem, Item> for T
where
    T: ?Sized,
    T: Sink<SinkItem, Error = E>,
    T: Stream<Item = Result<Item, E>>,
    E: Send + Sync + 'static,
{
}

#[pin_project]
pub struct ChannelTransport<SinkItem, Item> {
    sender: mpsc::UnboundedSender<SinkItem>,
    #[pin]
    receiver: mpsc::UnboundedReceiver<Item>,
}

impl<SinkItem, Item> ChannelTransport<SinkItem, Item> {
    pub fn unbounded() -> (ChannelTransport<SinkItem, Item>, ChannelTransport<Item, SinkItem>) {
        let (send_sink_item, recv_sink_item) = mpsc::unbounded_channel();
        let (send_item, recv_item) = mpsc::unbounded_channel();
        (
            ChannelTransport {
                sender: send_sink_item,
                receiver: recv_item,
            },
            ChannelTransport {
                sender: send_item,
                receiver: recv_sink_item,
            },
        )
    }
}

impl<SinkItem, Item> Sink<SinkItem> for ChannelTransport<SinkItem, Item> {
    type Error = io::Error;

    fn poll_ready(self: Pin<&mut Self>, _cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        if self.sender.is_closed() {
            Poll::Ready(Err(io::ErrorKind::BrokenPipe.into()))
        } else {
            Poll::Ready(Ok(()))
        }
    }

    fn start_send(self: Pin<&mut Self>, item: SinkItem) -> io::Result<()> {
        self.sender.send(item).map_err(|_| io::ErrorKind::BrokenPipe.into())
    }

    fn poll_flush(self: Pin<&mut Self>, _cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }

    fn poll_close(self: Pin<&mut Self>, _cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
}

impl<SinkItem, Item> Stream for ChannelTransport<SinkItem, Item> {
    type Item = io::Result<Item>;

    fn poll_next(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        Poll::Ready(ready!(self.project().receiver.poll_recv(cx)).map(|x| Ok(x)))
    }
}

trait Logit<T, E: fmt::Debug> {
    fn logit(self, msg: impl fmt::Display) -> Result<T, E>;
    fn with_logit<F, C>(self, f: F) -> Result<T, E>
    where
        C: fmt::Display,
        F: FnOnce() -> C;
}

impl<T, E: fmt::Debug> Logit<T, E> for Result<T, E> {
    fn logit(self, msg: impl fmt::Display) -> Result<T, E> {
        self.map_err(|e| {
            tracing::error!(error = ?e, "{}", msg);
            e
        })
    }

    fn with_logit<F, C>(self, f: F) -> Result<T, E>
    where
        C: fmt::Display,
        F: FnOnce() -> C,
    {
        self.logit(f())
    }
}

#[derive(Debug, Default, PartialEq, Eq, Clone)]
pub struct Dsn(mdsn::Dsn);

impl Deref for Dsn {
    type Target = mdsn::Dsn;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl DerefMut for Dsn {
    fn deref_mut(&mut self) -> &mut Self::Target {
        &mut self.0
    }
}

impl FromStr for Dsn {
    type Err = mdsn::DsnError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        return Ok(Dsn(mdsn::Dsn::from_str(s)?));
    }
}

type DynTransport<SinkItem, Item> =
    Serded<TokioFramed<DynReconnect<Dsn>, framed::DynCodec>, serded::DynSerde<Item, SinkItem>, Item, SinkItem>;

fn dyn_transport<SinkItem, Item>(dsn: &Dsn) -> io::Result<DynTransport<SinkItem, Item>> {
    Ok(reconnect::connect::<BoxReadWriter, _, _>(dsn.clone(), dyn_retry_iter(dsn))
        .framed(framed::DynCodec::new("xx")?)
        .serded::<_, Item, SinkItem>(serded::DynSerde::<Item, SinkItem>::new("xx")?))
}

fn transport(dsn: &Dsn) {
    let x = dsn.driver.split('-').size_hint();
}

#[cfg(test)]
mod tests {
    use crate::Dsn;

    #[test]
    fn test() {

    }
}
