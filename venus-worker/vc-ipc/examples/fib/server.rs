use std::io;

use tokio::net::TcpListener;
use tokio_util::codec::LengthDelimitedCodec;
use tower::service_fn;
use vc_ipc::{framed::FramedExt, readwriter::ReadWriterExt, serded::Bincode, server::Server};

pub fn fibonacci(n: usize) -> u64 {
    if n <= 1 {
        return 1;
    }

    let mut sum = 0;
    let mut last = 0;
    let mut curr = 1;
    for _i in 1..n {
        sum = last + curr;
        last = curr;
        curr = sum;
    }
    sum
}

#[tokio::main]
async fn main() -> io::Result<()> {
    let listener = TcpListener::bind("0.0.0.0:8964").await?;
    loop {
        match listener.accept().await {
            Ok((tcp_stream, _)) => {
                let tp = tcp_stream.framed(LengthDelimitedCodec::default()).serded(Bincode::default());
                let server: Server<_, _, _> = (tp, service_fn(|n: usize| async move { Ok::<_, u64>(fibonacci(n)) })).into();
                tokio::spawn(server.serve());
            }
            Err(e) => println!("couldn't get client: {:?}", e),
        }
    }
}
