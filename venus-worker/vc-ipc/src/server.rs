use std::{
    fmt::Debug,
    marker::PhantomData,
    pin::Pin,
    task::{Context, Poll},
};

use futures::{
    future::{self, ready, Join, Map, Ready},
    ready,
    stream::Fuse,
    Future, FutureExt, Sink, Stream, StreamExt, TryFutureExt, TryStream, TryStreamExt,
};
use tokio::sync::mpsc::{unbounded_channel, UnboundedSender};
use tokio_stream::wrappers::UnboundedReceiverStream;
use tower::{Service, ServiceExt};

use crate::{Logit, Request, Response, Transport};

impl<Req, Resp, TP, SVC> From<(TP, SVC)> for Server<Request<Req>, TP, IpcService<SVC>>
where
    TP: Transport<Response<Resp>, Request<Req>>,
    TP::Error: Debug,
    Req: Send + 'static,
    SVC: Service<Req, Response = Resp> + Clone + Send + 'static,
    SVC::Response: Send,
    SVC::Error: Send + Debug,
    SVC::Future: Send,
{
    fn from((transport, service): (TP, SVC)) -> Self {
        Server::new(transport, IpcService(service))
    }
}

#[derive(Clone)]
pub struct IpcService<S>(pub S);

impl<Req, S> tower::Service<Request<Req>> for IpcService<S>
where
    S: Service<Req>,
    S::Error: Debug,
{
    type Response = Response<S::Response>;

    type Error = S::Error;

    type Future = Map<Join<S::Future, Ready<u64>>, fn((<S::Future as Future>::Output, u64)) -> Result<Self::Response, Self::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(ready!(self.0.poll_ready(cx)).logit("poll_ready for service"))
    }

    fn call(&mut self, req: Request<Req>) -> Self::Future {
        future::join(self.0.call(req.body), ready(req.id)).map(|(result, request_id)| {
            result
                .map(|resp| Response {
                    id: request_id,
                    body: resp,
                })
                .logit("call service")
        })
    }
}

pub struct Server<Req, TP, S: Service<Req>> {
    transport: TP,
    service: S,
    _maker: PhantomData<Req>,
}

impl<Req, TP, S> Server<Req, TP, S>
where
    Req: Send + 'static,
    TP: Transport<S::Response, Req>,
    TP::Error: Debug,
    S: Service<Req> + Clone + Send + 'static,
    S::Response: Send,
    S::Future: Send,
    S::Error: Send + Debug,
{
    pub fn new(transport: TP, service: S) -> Self {
        Self {
            transport,
            service,
            _maker: PhantomData,
        }
    }

    pub async fn serve(self) -> Result<(), Error<S::Error, TP::Error>> {
        let (response_sender, response_recv) = unbounded_channel();
        let mut response_stream = UnboundedReceiverStream::new(response_recv);
        let (mut tp_sink, tp_stream) = self.transport.split();

        tokio::select! {
            result = WriteFut::new(&mut tp_sink, &mut response_stream) => {
                result
            }
            transport_result = read(tp_stream, self.service, response_sender) => {
                transport_result.map_err(Error::Transport)
            }
        }
    }
}

struct WriteFut<'a, TP, RS>
where
    TP: ?Sized,
    RS: TryStream + ?Sized,
{
    transport: &'a mut TP,
    response_stream: Fuse<&'a mut RS>,
    buffered: Option<RS::Ok>,
}

impl<'a, Resp, TP, RS> WriteFut<'a, TP, RS>
where
    TP: Sink<Resp> + Unpin + ?Sized,
    RS: TryStream<Ok = Resp> + Stream + Unpin + ?Sized,
{
    pub fn new(transport: &'a mut TP, response_stream: &'a mut RS) -> Self {
        Self {
            transport,
            response_stream: response_stream.fuse(),
            buffered: None,
        }
    }

    fn try_start_send(&mut self, cx: &mut Context<'_>, response: Resp) -> Poll<Result<(), TP::Error>> {
        debug_assert!(self.buffered.is_none());
        match Pin::new(&mut self.transport).poll_ready(cx)? {
            Poll::Ready(()) => Poll::Ready(Pin::new(&mut self.transport).start_send(response)),
            Poll::Pending => {
                self.buffered = Some(response);
                Poll::Pending
            }
        }
    }
}

// Pinning is never projected to any fields
impl<TP, RS> Unpin for WriteFut<'_, TP, RS>
where
    TP: Unpin + ?Sized,
    RS: TryStream + Unpin + ?Sized,
{
}

impl<Resp, TP, RS, SE> Future for WriteFut<'_, TP, RS>
where
    TP: Sink<Resp> + Unpin + ?Sized,
    RS: Stream<Item = Result<Resp, SE>> + Unpin + ?Sized,
    SE: Debug,
    TP::Error: Debug,
{
    type Output = Result<(), Error<SE, TP::Error>>;

    fn poll(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let this = &mut *self;
        // If we've got an item buffered already, we need to write it to the
        // sink before we can do anything else
        if let Some(item) = this.buffered.take() {
            ready!(this.try_start_send(cx, item)).map_err(Error::Transport)?
        }

        loop {
            match this.response_stream.try_poll_next_unpin(cx).map_err(Error::Service)? {
                Poll::Ready(Some(item)) => ready!(this.try_start_send(cx, item)).map_err(Error::Transport)?,
                Poll::Ready(None) => {
                    ready!(Pin::new(&mut this.transport).poll_flush(cx)).map_err(Error::Transport)?;
                    return Poll::Ready(Ok(()));
                }
                Poll::Pending => {
                    ready!(Pin::new(&mut this.transport).poll_flush(cx)).map_err(Error::Transport)?;
                    return Poll::Pending;
                }
            }
        }
    }
}

#[derive(thiserror::Error, Debug)]
pub enum Error<SE: Debug, TE: Debug> {
    /// The client disconnected from the server.
    #[error("service error: {0:?}")]
    Service(SE),
    #[error("transport error: {0:?}")]
    Transport(TE),
}

async fn read<Req, TP, S, TE>(
    mut transport: TP,
    service: S,
    response_sender: UnboundedSender<Result<S::Response, S::Error>>,
) -> Result<(), TE>
where
    Req: Send + 'static,
    TP: Stream<Item = Result<Req, TE>> + Unpin,
    S: Service<Req> + Clone + Send + 'static,
    S::Response: Send,
    S::Error: Send,
    S::Future: Send,
{
    while let Some(req) = transport.try_next().await? {
        let mut svc = service.clone();
        let response_sender = response_sender.clone();
        tokio::spawn(async move { response_sender.send(svc.ready().and_then(move |s| s.call(req)).await) });
    }
    Ok(())
}

#[cfg(test)]
mod tests {

    use std::convert::Infallible;

    use tower::{service_fn, ServiceBuilder};

    use crate::{client::Client, ChannelTransport, Request, Response};

    use super::{IpcService, Server};

    #[tokio::test]
    async fn test_server() {
        let (client, server_transport) = client();
        let svc = IpcService(service_fn(|req| async move { Ok::<_, Infallible>(format!("hello {}", req)) }));
        let svc = ServiceBuilder::new().buffer(10).service(svc);
        tokio::spawn(Server::new(server_transport, svc).serve());

        let resp = client.call("world".to_string()).await.unwrap();
        assert_eq!(resp, "hello world".to_string());

        let resp = client.call("lbw".to_string()).await.unwrap();
        assert_eq!(resp, "hello lbw".to_string());
    }

    fn client<Req, Resp>() -> (Client<Req, Resp>, ChannelTransport<Response<Resp>, Request<Req>>)
    where
        Req: Send + 'static,
        Resp: Send + 'static,
    {
        let (client_transport, server_transport) = ChannelTransport::unbounded();
        (Client::new(Default::default(), client_transport).spawn(), server_transport)
    }
}
