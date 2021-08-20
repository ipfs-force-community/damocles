use std::collections::VecDeque;
use std::future::Future;
use std::ops::Deref;
use std::pin::Pin;
use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Duration;

use async_std::{future::ready, prelude::FutureExt};
use jsonrpc_core::{
    futures::channel::mpsc::{unbounded, UnboundedReceiver},
    BoxFuture, MetaIoHandler, Middleware,
};
use jsonrpc_pubsub::Session;

use super::{duplex::Duplex, Client};
use crate::{RpcChannel, RpcError, RpcResult};

/// Implements a rpc client for `MetaIoHandler`.
pub struct LocalRpc<THandler> {
    handler: Arc<THandler>,
    meta: Arc<Session>,
    session_rx: UnboundedReceiver<String>,
    pending: VecDeque<BoxFuture<Option<String>>>,
}

impl<THandler, TMiddleware> LocalRpc<THandler>
where
    TMiddleware: Middleware<Arc<Session>>,
    THandler: Deref<Target = MetaIoHandler<Arc<Session>, TMiddleware>>,
{
    /// Creates a new `LocalRpc` with given handler and metadata.
    pub fn with_metadata(
        handler: Arc<THandler>,
        meta: Arc<Session>,
        rx: UnboundedReceiver<String>,
    ) -> Self {
        Self {
            handler,
            meta,
            session_rx: rx,
            pending: VecDeque::new(),
        }
    }
}

impl<THandler, TMiddleware> Client for LocalRpc<THandler>
where
    TMiddleware: Middleware<Arc<Session>>,
    THandler: Deref<Target = MetaIoHandler<Arc<Session>, TMiddleware>> + Unpin + 'static,
{
    type ConnectInfo = Arc<THandler>;
    type ConnectError = RpcError;

    fn connect(
        info: &Self::ConnectInfo,
        dealy: Option<Duration>,
    ) -> Pin<Box<dyn Future<Output = Result<Self, Self::ConnectError>>>> {
        let (tx, rx) = unbounded();

        let meta = Arc::new(Session::new(tx));

        let fut = ready(Ok(Self::with_metadata(info.clone(), meta, rx)));

        match dealy {
            Some(d) => Box::pin(fut.delay(d)),
            None => Box::pin(fut),
        }
    }

    fn handle_stream(
        &mut self,
        _cx: &mut Context<'_>,
        incoming: &mut VecDeque<String>,
    ) -> Result<(), String> {
        while let Ok(Some(s)) = self.session_rx.try_next() {
            incoming.push_back(s);
        }

        let drained = self.pending.drain(..).collect::<Vec<_>>();
        for mut item in drained {
            match Pin::new(&mut item).poll(_cx) {
                Poll::Ready(Some(s)) => {
                    incoming.push_back(s);
                }

                Poll::Ready(None) => {}

                Poll::Pending => self.pending.push_back(item),
            }
        }

        Ok(())
    }

    fn handle_sink(
        &mut self,
        _cx: &mut Context<'_>,
        outgoing: &mut VecDeque<String>,
    ) -> Result<bool, String> {
        let drained = outgoing.drain(..);
        for item in drained {
            let future = self.handler.handle_request(&item, self.meta.clone());
            self.pending.push_back(Box::pin(future));
        }

        Ok(true)
    }
}

pub async fn connect<TClient, THandler>(
    done: crossbeam_channel::Receiver<()>,
    handler: THandler,
) -> RpcResult<(TClient, impl Future<Output = RpcResult<()>>)>
where
    TClient: From<RpcChannel>,
    THandler: Deref<Target = MetaIoHandler<Arc<Session>>> + Unpin + 'static,
{
    let hdl = Arc::new(handler);
    let (fut, tx) = Duplex::<LocalRpc<THandler>>::new(done, hdl).await?;
    Ok((TClient::from(tx), fut))
}
