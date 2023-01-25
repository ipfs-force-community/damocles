use std::{io, pin::Pin};

use futures::future::BoxFuture;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_util::codec::Framed as TokioFramed;

use crate::Dsn;

use self::reconnect::Reconnectable;

pub mod pipe;
pub mod reconnect;
pub mod tcp;

impl<T: ?Sized> ReadWriter for T where T: AsyncRead + AsyncWrite {}

pub trait ReadWriter: AsyncRead + AsyncWrite {}

impl<T: ?Sized> ReadWriterExt for T where T: ReadWriter {}

pub trait ReadWriterExt: ReadWriter {
    fn framed<Codec>(self, frame_codec: Codec) -> TokioFramed<Self, Codec>
    where
        Self: Sized,
    {
        TokioFramed::new(self, frame_codec)
    }
}

pub type BoxReadWriter = Pin<Box<dyn ReadWriter>>;

async fn connect_dyn(dsn: Dsn) -> io::Result<BoxReadWriter> {
    todo!()
}

impl Reconnectable<Dsn> for BoxReadWriter {
    type ConnectingFut = BoxFuture<'static, io::Result<BoxReadWriter>>;

    fn connect(ctor_arg: Dsn) -> Self::ConnectingFut {
        Box::pin(connect_dyn(ctor_arg))
    }
}
