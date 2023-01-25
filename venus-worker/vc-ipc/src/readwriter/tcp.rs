use std::{
    io,
    net::{SocketAddr, ToSocketAddrs},
    vec,
};

use futures::future::BoxFuture;
use tokio::net::TcpStream;

use crate::Dsn;

use super::reconnect::Reconnectable;

impl<A> Reconnectable<A> for TcpStream
where
    A: ToSocketAddrs + Send + 'static,
{
    type ConnectingFut = BoxFuture<'static, io::Result<TcpStream>>;

    fn connect(ctor_arg: A) -> Self::ConnectingFut {
        Box::pin(async move {
            let socket_addrs = ctor_arg.to_socket_addrs()?.collect::<Vec<_>>();
            TcpStream::connect(socket_addrs.as_slice()).await
        })
    }
}

impl ToSocketAddrs for Dsn {
    type Iter = vec::IntoIter<SocketAddr>;

    fn to_socket_addrs(&self) -> io::Result<Self::Iter> {
        self.addresses
            .iter()
            .map(|addr| match (&addr.host, addr.port) {
                (Some(host), Some(port)) => Ok(SocketAddr::new(
                    host.parse().map_err(|e| io::Error::new(io::ErrorKind::AddrNotAvailable, e))?,
                    port,
                )),
                (Some(host), None) => host.parse().map_err(|e| io::Error::new(io::ErrorKind::AddrNotAvailable, e)),
                _ => Err(io::ErrorKind::AddrNotAvailable.into()),
            })
            .collect::<io::Result<Vec<_>>>()
            .map(IntoIterator::into_iter)
    }
}
