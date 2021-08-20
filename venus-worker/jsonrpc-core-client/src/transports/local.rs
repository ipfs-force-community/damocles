use std::collections::VecDeque;
use std::future::Future;
use std::ops::Deref;
use std::pin::Pin;
use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Duration;

use async_std::{
    future::ready,
    prelude::FutureExt,
    task::{block_on, spawn},
};
use jsonrpc_core::{
    futures::channel::mpsc::UnboundedReceiver, BoxFuture, MetaIoHandler, Metadata, Middleware,
};
use tracing::{error, info};

use super::{duplex::Duplex, Client};
use crate::{RpcChannel, RpcError, RpcResult};

/// Implements a rpc client for `MetaIoHandler`.
pub struct LocalRpc<THandler, TMetadata> {
    handler: Arc<THandler>,
    meta: TMetadata,
    session_rx: Option<UnboundedReceiver<String>>,
    pending: VecDeque<BoxFuture<Option<String>>>,
}

impl<THandler, TMetadata, TMiddleware> LocalRpc<THandler, TMetadata>
where
    TMetadata: Metadata,
    TMiddleware: Middleware<TMetadata>,
    THandler: Deref<Target = MetaIoHandler<TMetadata, TMiddleware>>,
{
    /// Creates a new `LocalRpc` with given handler and metadata.
    pub fn new(handler: Arc<THandler>, meta: TMetadata) -> Self {
        Self {
            handler,
            meta,
            session_rx: None,
            pending: VecDeque::new(),
        }
    }
}

impl<THandler, TMetadata, TMiddleware> Client for LocalRpc<THandler, TMetadata>
where
    TMetadata: Metadata + Default + Unpin,
    TMiddleware: Middleware<TMetadata>,
    THandler: Deref<Target = MetaIoHandler<TMetadata, TMiddleware>> + Send + Sync + Unpin + 'static,
{
    type ConnectInfo = Arc<THandler>;
    type ConnectError = RpcError;

    fn connect(
        info: &Self::ConnectInfo,
        dealy: Option<Duration>,
    ) -> Pin<Box<dyn Future<Output = Result<Self, Self::ConnectError>> + Send>> {
        let fut = ready(Ok(Self::new(info.clone(), Default::default())));

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
        if let Some(rx) = self.session_rx.as_mut() {
            while let Ok(Some(s)) = rx.try_next() {
                incoming.push_back(s);
            }
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

pub fn connect<TClient, TMetadata, THandler>(handler: THandler) -> RpcResult<TClient>
where
    TClient: From<RpcChannel>,
    THandler: Deref<Target = MetaIoHandler<TMetadata>> + Unpin + Send + Sync + 'static,
    TMetadata: Metadata + Default + Unpin,
{
    let hdl = Arc::new(handler);
    let (duplex, tx) = block_on(Duplex::<LocalRpc<THandler, TMetadata>>::new(hdl))?;
    spawn(async move {
        if let Err(e) = duplex.await {
            error!("duplex shutdown unexpectedlly: {:?}", e);
        } else {
            info!("duplex shutdown")
        }
    });
    Ok(TClient::from(tx))
}
