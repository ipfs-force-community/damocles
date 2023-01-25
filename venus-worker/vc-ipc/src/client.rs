use std::{
    error::Error as StdError,
    fmt::Debug,
    future::Future,
    mem,
    pin::Pin,
    sync::atomic::{AtomicU64, Ordering},
    task::{Context, Poll},
    time::{Duration, Instant},
};

use futures::{ready, stream::Fuse, Sink, Stream, StreamExt};
use pin_project::pin_project;
use tokio::sync::{mpsc, oneshot};

use self::inflight_requests::{DeadlineExceededError, InflightRequests};
use crate::{Request, Response, Transport, TransportError};

#[derive(thiserror::Error, Debug)]
pub enum IpcError {
    /// The client disconnected from the server.
    #[error("the client disconnected from the server")]
    Disconnected,

    /// The request exceeded its deadline.
    #[error("the request exceeded its deadline")]
    DeadlineExceeded,
}

pub struct Config {
    pub max_pending_requests: usize,
    pub max_inflight_requests: usize,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            max_pending_requests: 100,
            max_inflight_requests: 1000,
        }
    }
}

pub struct Client<Req, Resp> {
    next_request_id: AtomicU64,
    send_requests: mpsc::Sender<DispatchRequest<Req, Resp>>,
}

impl<Req, Resp> Client<Req, Resp> {
    pub fn new<TP>(config: Config, transport: TP) -> NewClient<Self, RequestDispatcher<Req, Resp, TP>>
    where
        TP: Transport<Request<Req>, Response<Resp>>,
    {
        let (send_requests, recv_requests) = mpsc::channel(config.max_pending_requests);

        NewClient {
            client: Client {
                next_request_id: AtomicU64::new(1),
                send_requests,
            },

            dispatch: RequestDispatcher {
                transport: transport.fuse(),
                pending_requests: recv_requests,
                inflight_requests: InflightRequests::new(),
                write_state: WriteState::PollPendingRequest { need_flush: false },
            },
        }
    }

    pub async fn call(&self, req: Req) -> Result<Resp, IpcError> {
        self.call_timeout(req, Duration::from_secs(60)).await
    }

    pub async fn call_timeout(&self, req: Req, timeout: Duration) -> Result<Resp, IpcError> {
        let (response_completion, wait_resp) = oneshot::channel();
        let next_request_id = self.next_request_id.fetch_add(1, Ordering::Relaxed);
        self.send_requests
            .send(DispatchRequest {
                deadline: Instant::now() + timeout,
                response_completion,
                request: Request {
                    id: next_request_id,
                    body: req,
                },
            })
            .await
            .map_err(|_| IpcError::Disconnected)?;

        let response = wait_resp
            .await
            .map_err(|_| IpcError::Disconnected)?
            .map_err(|_| IpcError::DeadlineExceeded)?;
        Ok(response.body)
    }
}

pub struct NewClient<C, D> {
    client: C,
    dispatch: D,
}

impl<C, D, E> NewClient<C, D>
where
    D: Future<Output = Result<(), E>> + Send + 'static,
    E: Send + Debug + 'static,
{
    pub fn spawn(self) -> C {
        let Self { client, dispatch } = self;
        tokio::spawn(async {
            if let Err(e) = dispatch.await {
                tracing::error!(err = ?e, "dispatch");
            }
        });
        client
    }
}

struct DispatchRequest<Req, Resp> {
    deadline: Instant,
    response_completion: oneshot::Sender<Result<Response<Resp>, DeadlineExceededError>>,
    request: Request<Req>,
}

#[pin_project]
pub struct RequestDispatcher<Req, Resp, TP: Transport<Request<Req>, Response<Resp>>> {
    #[pin]
    transport: Fuse<TP>,
    pending_requests: mpsc::Receiver<DispatchRequest<Req, Resp>>,
    inflight_requests: InflightRequests<Resp>,
    write_state: WriteState<Req, Resp, TP::Error>,
}

enum WriteState<Req, Resp, TE> {
    PollPendingRequest { need_flush: bool },
    WritingRequest(DispatchRequest<Req, Resp>),
    Flushing,
    Closing(Option<TE>),
}

impl<Req, Resp, TE> WriteState<Req, Resp, TE> {
    fn take(&mut self, next_state: Self) -> Self {
        mem::replace(self, next_state)
    }
}

impl<Req, Resp, TP> RequestDispatcher<Req, Resp, TP>
where
    TP: Transport<Request<Req>, Response<Resp>>,
    TP::Error: Debug,
{
    fn poll_read(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        loop {
            if ready!(self
                .as_mut()
                .transport()
                .poll_next(cx)
                .map_ok(|resp| self.as_mut().inflight_requests().complete_request(resp))?)
            .is_none()
            {
                return Poll::Ready(Ok(()));
            }
        }
    }

    fn poll_write(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        loop {
            match self.as_mut().project().write_state.take(WriteState::Closing(None)) {
                WriteState::PollPendingRequest { need_flush } => {
                    *self.as_mut().project().write_state = match self.as_mut().project().pending_requests.poll_recv(cx) {
                        Poll::Ready(Some(dispatch_request)) => {
                            if dispatch_request.response_completion.is_closed() {
                                // TODO: span.enter();
                                tracing::info!("AbortRequest");
                                WriteState::PollPendingRequest { need_flush }
                            } else {
                                WriteState::WritingRequest(dispatch_request)
                            }
                        }
                        Poll::Ready(None) => WriteState::Closing(None),
                        Poll::Pending => {
                            if need_flush {
                                WriteState::Flushing
                            } else {
                                self.as_mut().set_write_state(WriteState::PollPendingRequest { need_flush: false });
                                return Poll::Pending;
                            }
                        }
                    }
                }
                WriteState::WritingRequest(r) => {
                    *self.as_mut().project().write_state = match self.as_mut().poll_ready(cx) {
                        Poll::Ready(Ok(_)) => match self.as_mut().write_request(r) {
                            Ok(_) => WriteState::PollPendingRequest { need_flush: true },
                            Err(err) => WriteState::Closing(Some(err)),
                        },
                        Poll::Ready(Err(err)) => WriteState::Closing(Some(err)),
                        Poll::Pending => {
                            self.as_mut().set_write_state(WriteState::WritingRequest(r));
                            return Poll::Pending;
                        }
                    }
                }
                WriteState::Flushing => {
                    *self.as_mut().project().write_state = match self.as_mut().poll_flush(cx) {
                        Poll::Ready(Ok(_)) => WriteState::PollPendingRequest { need_flush: false },
                        Poll::Ready(Err(e)) => WriteState::Closing(Some(e)),
                        Poll::Pending => {
                            self.as_mut().set_write_state(WriteState::Flushing);
                            return Poll::Pending;
                        }
                    }
                }
                WriteState::Closing(err_opt) => {
                    let inflight_requests = self.as_mut().inflight_requests();
                    if !inflight_requests.is_empty() {
                        tracing::info!(
                            "Shutdown: write half closed, and {} requests in flight.",
                            self.inflight_requests.len()
                        );
                        self.as_mut().set_write_state(WriteState::Closing(err_opt));
                        return Poll::Pending;
                    }
                    tracing::info!("Shutdown: write half closed, and no requests in flight.");
                    match self.as_mut().poll_close(cx) {
                        Poll::Ready(Ok(_)) => {}
                        Poll::Ready(Err(poll_close_err)) => {
                            return Poll::Ready(Err(match err_opt {
                                Some(err) => {
                                    tracing::error!(err=?poll_close_err, "poll_close");
                                    err
                                }
                                None => poll_close_err,
                            }))
                        }
                        Poll::Pending => {
                            self.as_mut().set_write_state(WriteState::Closing(err_opt));
                            return Poll::Pending;
                        }
                    }
                    return Poll::Ready(match err_opt {
                        Some(err) => Err(err),
                        None => Ok(()),
                    });
                }
            }
        }
    }

    fn poll_expired(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<()> {
        loop {
            match ready!(self.as_mut().inflight_requests().poll_expired(cx)) {
                Some(expired) => {
                    tracing::trace!("request: {} expired", expired);
                }
                None => {
                    return Poll::Ready(());
                }
            }
        }
    }

    fn set_write_state(self: Pin<&mut Self>, new_write_state: WriteState<Req, Resp, TP::Error>) {
        *self.project().write_state = new_write_state;
    }

    fn poll_ready(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        self.transport().poll_ready(cx)
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        self.transport().poll_flush(cx)
    }

    fn write_request(mut self: Pin<&mut Self>, dispatch_request: DispatchRequest<Req, Resp>) -> Result<(), TP::Error> {
        let request_id = dispatch_request.request.id;
        self.as_mut().start_send(dispatch_request.request)?;
        self.as_mut()
            .inflight_requests()
            .insert_request(request_id, dispatch_request.deadline, dispatch_request.response_completion);
        Ok(())
    }

    fn start_send(self: Pin<&mut Self>, request: Request<Req>) -> Result<(), TP::Error> {
        self.transport().start_send(request)
    }

    fn poll_close(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), TP::Error>> {
        self.transport().poll_close(cx)
    }

    fn transport(self: Pin<&mut Self>) -> Pin<&mut Fuse<TP>> {
        self.project().transport
    }

    fn inflight_requests(self: Pin<&mut Self>) -> &mut InflightRequests<Resp> {
        self.project().inflight_requests
    }
}

impl<Req, Resp, TP> Future for RequestDispatcher<Req, Resp, TP>
where
    TP: Transport<Request<Req>, Response<Resp>>,
    TP::Error: Debug,
{
    type Output = Result<(), TP::Error>;

    fn poll(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        match (
            self.as_mut().poll_read(cx)?,
            self.as_mut().poll_write(cx)?,
            self.as_mut().poll_expired(cx),
        ) {
            (Poll::Ready(_), _, _) | (_, Poll::Ready(_), _) => Poll::Ready(Ok(())),
            _ => Poll::Pending,
        }
    }
}

mod inflight_requests {
    use std::{
        collections::hash_map::Entry,
        task::{Context, Poll},
        time::Instant,
    };

    use fnv::FnvHashMap;
    use tokio::sync::oneshot;
    use tokio_util::time::{delay_queue, DelayQueue};

    use crate::Response;

    /// The request exceeded its deadline.
    #[derive(thiserror::Error, Debug)]
    #[non_exhaustive]
    #[error("the request exceeded its deadline")]
    pub struct DeadlineExceededError;

    struct RequestData<Resp> {
        response_completion: oneshot::Sender<Result<Response<Resp>, DeadlineExceededError>>,
        /// The key to remove the timer for the request's deadline.
        deadline_key: delay_queue::Key,
    }

    /// Wrap FnvHashMap to make pin work
    pub struct InflightRequests<Resp> {
        requests: FnvHashMap<u64, RequestData<Resp>>,
        deadlines: DelayQueue<u64>,
    }

    impl<Resp> InflightRequests<Resp> {
        pub fn new() -> Self {
            Self {
                requests: Default::default(),
                deadlines: Default::default(),
            }
        }

        pub fn is_empty(&self) -> bool {
            self.requests.is_empty()
        }

        pub fn len(&self) -> usize {
            self.requests.len()
        }

        pub fn insert_request(
            &mut self,
            request_id: u64,
            deadline: Instant,
            response_completion: oneshot::Sender<Result<Response<Resp>, DeadlineExceededError>>,
        ) {
            match self.requests.entry(request_id) {
                Entry::Vacant(vacant) => {
                    let deadline_key = self.deadlines.insert_at(request_id, deadline.into());
                    vacant.insert(RequestData {
                        response_completion,
                        deadline_key,
                    });
                }
                Entry::Occupied(_) => {}
            }
        }

        pub fn complete_request(&mut self, resp: Response<Resp>) {
            if let Some(request_data) = self.requests.remove(&resp.id) {
                self.requests.shrink_to_fit();
                self.deadlines.remove(&request_data.deadline_key);
                let _ = request_data.response_completion.send(Ok(resp));
            }
        }

        pub fn poll_expired(&mut self, cx: &mut Context) -> Poll<Option<u64>> {
            self.deadlines.poll_expired(cx).map(|expired| {
                let request_id = expired?.into_inner();
                if let Some(request_data) = self.requests.remove(&request_id) {
                    // let _entered = request_data.span.enter();
                    tracing::error!("DeadlineExceeded");
                    self.requests.shrink_to_fit();
                    let _ = request_data.response_completion.send(Err(DeadlineExceededError));
                }
                Some(request_id)
            })
        }
    }
}

#[cfg(test)]
mod tests {

    use std::convert::Infallible;

    use tokio::time::sleep;
    use tower::service_fn;

    use crate::{server::Server, ChannelTransport};

    use super::*;

    #[tokio::test]
    async fn test_client_call() {
        let tp = server(service_fn(
            |req: String| async move { Ok::<_, Infallible>(format!("hello {}", req)) },
        ));
        let client = Client::new(Default::default(), tp).spawn();

        let resp = client.call("world".to_string()).await.unwrap();
        assert_eq!(resp, "hello world".to_string());

        let resp = client.call("lbw".to_string()).await.unwrap();
        assert_eq!(resp, "hello lbw".to_string());
    }

    #[tokio::test]
    async fn test_client_request_deadline() {
        let timeout = Duration::from_secs(1);
        let tp = server(service_fn(move |req: String| async move {
            sleep(timeout).await;
            Ok::<_, Infallible>(format!("hello {}", req))
        }));
        let client = Client::new(Default::default(), tp).spawn();

        assert!(matches!(
            client.call_timeout("world".to_string(), Duration::from_millis(100)).await,
            Err(IpcError::DeadlineExceeded)
        ));

        let resp = client
            .call_timeout("lbw".to_string(), timeout + Duration::from_millis(100))
            .await
            .unwrap();
        assert_eq!(resp, "hello lbw".to_string());
    }

    fn server<Req, S>(service: S) -> ChannelTransport<Request<Req>, Response<S::Response>>
    where
        Req: Send + 'static,
        S: tower::Service<Req> + Clone + Send + Sync + 'static,
        S::Response: Send,
        S::Error: Send + Debug,
        S::Future: Send,
    {
        let (client_transport, server_transport) = ChannelTransport::unbounded();
        let server: Server<_, _, _> = (server_transport, service).into();

        tokio::spawn(server.serve());
        client_transport
    }
}
