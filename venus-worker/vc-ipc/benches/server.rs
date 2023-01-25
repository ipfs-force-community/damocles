use std::{
    fmt::Debug,
    io,
    task::{Context, Poll},
};

use criterion::{black_box, criterion_group, criterion_main, Criterion};
use futures::{
    future::{ready, Ready},
    SinkExt, StreamExt, TryFutureExt,
};
use tokio::{runtime::Runtime, sync::mpsc, try_join};
use tower::{BoxError, Service, ServiceExt};
use vc_ipc::{
    client::Client,
    server::{IpcService, Server},
    ChannelTransport, Request, Response,
};

#[derive(Clone)]
struct MyService;

impl tower::Service<String> for MyService {
    type Response = String;

    type Error = String;

    type Future = Ready<Result<Self::Response, Self::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn call(&mut self, req: String) -> Self::Future {
        ready(Ok(format!("hello {}", req)))
    }
}

fn bench_server_future(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();
    let _guard = rt.enter();

    let client_transport = server_future(MyService, 100);

    let client = Client::new(Default::default(), client_transport).spawn();

    c.bench_function("bench server future", |b| {
        b.to_async(&rt)
            .iter(|| async { black_box(client.call(black_box("world".to_string())).await) });
    });
}

fn bench_server_async(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();
    let _guard = rt.enter();

    let client_transport = server_async(MyService, 100);
    let client = Client::new(Default::default(), client_transport).spawn();

    c.bench_function("bench server async", |b| {
        b.to_async(&rt)
            .iter(|| async { black_box(client.call(black_box("world".to_string())).await) });
    });
}

fn server_future<Req, S>(service: S, bound: usize) -> ChannelTransport<Request<Req>, Response<S::Response>>
where
    Req: Send + 'static,
    S: Service<Req> + Clone + Send + Sync + 'static,
    S::Response: Send,
    S::Future: Send,
    S::Error: Send + Sync + Into<BoxError>,
{
    let (client_transport, server_transport) = ChannelTransport::unbounded();
    let svc = tower::ServiceBuilder::new().buffer(bound).service(service);
    let server: Server<_, _, _> = (server_transport, svc).into();
    tokio::spawn(server.serve());
    client_transport
}

fn server_async<Req, S>(service: S, bound: usize) -> ChannelTransport<Request<Req>, Response<S::Response>>
where
    Req: Send + 'static,
    S: Service<Req> + Clone + Send + Sync + 'static,
    S::Response: Send,
    S::Future: Send,
    S::Error: Send + Sync + Into<BoxError> + Debug,
{
    let (client_transport, server_transport) = ChannelTransport::<Request<Req>, _>::unbounded();

    let svc = tower::ServiceBuilder::new().buffer(bound).service(service);

    tokio::spawn(async move {
        let (mut sink, mut stream) = server_transport.split();

        let (send_response, mut recv_response) = mpsc::unbounded_channel();

        let handle_requests = async move {
            loop {
                match stream.next().await {
                    Some(Ok(request)) => {
                        let mut service_cloned = svc.clone();
                        let send_response_cloned = send_response.clone();
                        tokio::spawn(async move {
                            let result = service_cloned.ready().and_then(move |s| s.call(request.body)).await;
                            send_response_cloned.send(result.map(|r| Response { id: request.id, body: r }))
                        });
                    }
                    Some(Err(error)) => return Err(error),
                    None => return Ok(()),
                }
            }
        };

        let send_responses = async move {
            while let Some(resp) = recv_response.recv().await {
                sink.send(resp.map_err(|e| io::Error::new(io::ErrorKind::Other, e))?).await?;
            }
            Ok::<_, io::Error>(())
        };

        if let Err(error) = try_join!(handle_requests, send_responses) {
            tracing::error!(err=?error, "server error");
        }
    });
    client_transport
}

criterion_group!(benches, bench_server_future, bench_server_async);
criterion_main!(benches);
