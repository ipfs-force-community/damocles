use std::{
    future::Future,
    io, iter,
    pin::Pin,
    task::{Context, Poll},
    time::Duration,
};

use futures::ready;
use pin_project::pin_project;
use tokio::{
    io::{AsyncRead, AsyncWrite, ReadBuf},
    time::{sleep, Sleep},
};

use crate::Dsn;

use super::BoxReadWriter;

pub trait Reconnectable<C>: Sized {
    type ConnectingFut: Future<Output = io::Result<Self>>;

    fn connect(ctor_arg: C) -> Self::ConnectingFut;

    fn is_disconnect_error(&self, err: &io::Error) -> bool {
        use std::io::ErrorKind::*;

        matches!(
            err.kind(),
            NotFound
                | PermissionDenied
                | ConnectionRefused
                | ConnectionReset
                | ConnectionAborted
                | NotConnected
                | AddrInUse
                | AddrNotAvailable
                | BrokenPipe
                | AlreadyExists
        )
    }

    fn is_read_disconnect_error(&self, err: &io::Error) -> bool {
        self.is_disconnect_error(err)
    }

    fn is_write_disconnect_error(&self, err: &io::Error) -> bool {
        self.is_disconnect_error(err)
    }
}

/// Connect to the underlying io object to make it automatically reconnects
///
/// # Examples
///
/// ```
/// use std::{error::Error, iter::repeat, path::Path, time::Duration};
///
/// use tokio::io::{AsyncReadExt, AsyncWriteExt};
/// use vc_ipc::readwriter::{reconnect::connect as reconnect, pipe::Pipe};
///
/// #[tokio::main]
/// async fn main() -> Result<(), Box<dyn Error>> {
///     let mut stream =
///         reconnect::<Pipe<_, _>, _, _>(Path::new("/bin/cat"), repeat(Duration::from_millis(500)));
///
///     // Write some data.
///     stream.write_all(b"hello world").await?;
///
///     let mut buf = vec![0; 11];
///     stream.read_exact(&mut buf).await?;
///     assert_eq!(b"hello world", buf.as_slice());
///     Ok(())
/// }
/// ```
pub fn connect<RW, C, RetryIt>(ctor_arg: C, retry_iter: RetryIt) -> Reconnect<RW, C, RetryIt>
where
    C: Clone,
    RW: Reconnectable<C>,
    RetryIt: Iterator,
{
    Reconnect {
        state: State::Connecting(RW::connect(ctor_arg.clone())),
        ctor_arg,
        retry_iter: retry_iter.enumerate(),
    }
}

pub fn dyn_retry_iter(dsn: &Dsn) -> Box<dyn Iterator<Item = Duration>> {
    todo!()
}

pub type DynReconnect<C> = Reconnect<BoxReadWriter, C, Box<dyn Iterator<Item = Duration>>>;

#[pin_project]
pub struct Reconnect<T, C, RetryIt>
where
    T: Reconnectable<C>,
{
    #[pin]
    state: State<T::ConnectingFut, T>,
    ctor_arg: C,
    retry_iter: iter::Enumerate<RetryIt>,
}

#[derive(Debug)]
#[pin_project(project = StateProj)]
enum State<CF, T> {
    Disconnected,
    WaitingForRetry(Pin<Box<Sleep>>),
    Connecting(#[pin] CF),
    Connected(#[pin] T),
    Exhausted,
}

impl<RW, C, RetryIt> Reconnect<RW, C, RetryIt>
where
    C: Clone,
    RW: Reconnectable<C>,
    RetryIt: Iterator<Item = Duration>,
{
    fn poll<F, R>(mut self: Pin<&mut Self>, cx: &mut Context<'_>, mut f: F) -> Poll<io::Result<R>>
    where
        F: FnMut(Pin<&mut RW>, &mut Context<'_>) -> Option<Poll<io::Result<R>>>,
    {
        loop {
            let next_state = match self.as_mut().project().state.project() {
                StateProj::Disconnected => match self.as_mut().project().retry_iter.next() {
                    Some((reconnect_num, sleep_dur)) => {
                        tracing::info!("Will re-perform initial connect attempt #{} in {:?}.", reconnect_num, sleep_dur);
                        State::WaitingForRetry(Box::pin(sleep(sleep_dur)))
                    }
                    None => State::Exhausted,
                },
                StateProj::WaitingForRetry(sleep_fut) => {
                    ready!(sleep_fut.as_mut().poll(cx));
                    State::Connecting(RW::connect(self.ctor_arg.clone()))
                }
                StateProj::Connecting(fut) => match ready!(fut.poll(cx)) {
                    Ok(rw) => {
                        tracing::info!("Initial connection successfully established.");
                        State::Connected(rw)
                    }
                    Err(err) => {
                        tracing::error!("Connecting failure: {}", err);
                        State::Disconnected
                    }
                },
                StateProj::Connected(rw) => match f(rw, cx) {
                    Some(r) => return r,
                    None => State::Disconnected,
                },
                StateProj::Exhausted => {
                    return Poll::Ready(Err(io::Error::new(
                        io::ErrorKind::NotConnected,
                        "Disconnected. Connection attempts have been exhausted.",
                    )));
                }
            };
            self.as_mut().project().state.set(next_state);
        }
    }
}

impl<T, C, RetryIt> AsyncRead for Reconnect<T, C, RetryIt>
where
    C: Clone,
    T: Reconnectable<C> + AsyncRead,
    RetryIt: Iterator<Item = Duration>,
{
    fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut ReadBuf<'_>) -> Poll<io::Result<()>> {
        self.poll(cx, |mut rw, cx| {
            let filled_before = buf.filled().len();
            let res = match rw.as_mut().poll_read(cx, buf) {
                Poll::Ready(res) => res,
                Poll::Pending => return Some(Poll::Pending),
            };
            let filled_after = buf.filled().len();

            // TODO(0x5459): HOW TO CHECK IT IS REALLY EOF
            // EOF or disconnect
            // if filled_after - filled_before == 0 {
            //     return None;
            // }

            match res {
                Err(err) if rw.is_read_disconnect_error(&err) => None,
                x => Some(Poll::Ready(x)),
            }
        })
    }
}

impl<T, C, RetryIt> AsyncWrite for Reconnect<T, C, RetryIt>
where
    C: Clone,
    T: Reconnectable<C> + AsyncWrite,
    RetryIt: Iterator<Item = Duration>,
{
    fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<io::Result<usize>> {
        self.poll(cx, |mut rw, cx| match rw.as_mut().poll_write(cx, buf) {
            Poll::Ready(Err(err)) if rw.is_write_disconnect_error(&err) => None,
            x => Some(x),
        })
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        self.poll(cx, |mut rw, cx| match rw.as_mut().poll_flush(cx) {
            Poll::Ready(Err(err)) if rw.is_write_disconnect_error(&err) => None,
            x => Some(x),
        })
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        self.poll(cx, |mut rw, cx| match rw.as_mut().poll_shutdown(cx) {
            Poll::Ready(Err(err)) if rw.is_write_disconnect_error(&err) => None,
            x => Some(x),
        })
    }
}

#[cfg(test)]
mod tests {
    use std::{
        io,
        iter::repeat,
        pin::Pin,
        sync::{Arc, Mutex as StdMutex},
        task::{Context, Poll},
        time::Duration,
    };

    use futures::future::{ready, Ready};
    use pin_project::pin_project;
    use rand::{random, Rng, RngCore};
    use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadBuf};

    use super::{connect, Reconnectable};

    #[pin_project]
    pub struct RandomDisconnect<T> {
        #[pin]
        inner: T,
    }

    fn random_disconnect() -> io::Result<()> {
        if random() {
            Ok(())
        } else {
            Err(io::ErrorKind::ConnectionAborted.into())
        }
    }

    impl<T: AsyncRead> AsyncRead for RandomDisconnect<T> {
        fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut ReadBuf<'_>) -> Poll<io::Result<()>> {
            random_disconnect()?;

            let want_read = rand::thread_rng().gen_range(1..=buf.remaining());
            let mut try_read_buf = buf.take(want_read);
            let filled_before = try_read_buf.filled().len();

            let poll_result = self.project().inner.poll_read(cx, &mut try_read_buf);

            let filled_after = try_read_buf.filled().len();
            let filled = filled_after - filled_before;
            unsafe { buf.assume_init(filled) };
            buf.advance(filled);
            poll_result
        }
    }

    impl<T: AsyncWrite> AsyncWrite for RandomDisconnect<T> {
        fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<io::Result<usize>> {
            random_disconnect()?;

            let want_write = rand::thread_rng().gen_range(1..=buf.len());
            let try_write_buf = &buf[..want_write];
            self.project().inner.poll_write(cx, try_write_buf)
        }

        fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
            random_disconnect()?;
            self.project().inner.poll_flush(cx)
        }

        fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
            random_disconnect()?;
            self.project().inner.poll_shutdown(cx)
        }
    }

    impl<T> Reconnectable<T> for RandomDisconnect<T> {
        type ConnectingFut = Ready<io::Result<Self>>;

        fn connect(inner: T) -> Self::ConnectingFut {
            ready(Ok(RandomDisconnect { inner }))
        }
    }

    #[derive(Clone)]
    pub struct LockedIO<T> {
        pub inner: Arc<StdMutex<T>>,
    }

    impl<T> LockedIO<T> {
        pub fn new(inner: T) -> Self {
            Self {
                inner: Arc::new(StdMutex::new(inner)),
            }
        }
    }

    impl<T: AsyncRead + Unpin> AsyncRead for LockedIO<T> {
        fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut ReadBuf<'_>) -> Poll<io::Result<()>> {
            Pin::new(&mut *self.inner.lock().unwrap()).poll_read(cx, buf)
        }
    }

    impl<T: AsyncWrite + Unpin> AsyncWrite for LockedIO<T> {
        fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<io::Result<usize>> {
            Pin::new(&mut *self.inner.lock().unwrap()).poll_write(cx, buf)
        }

        fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), io::Error>> {
            Pin::new(&mut *self.inner.lock().unwrap()).poll_flush(cx)
        }

        fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), io::Error>> {
            Pin::new(&mut *self.inner.lock().unwrap()).poll_shutdown(cx)
        }
    }

    #[tokio::test]
    async fn test_reconnect_read() {
        let expect_data = random_data();
        let mut r = connect::<RandomDisconnect<_>, _, _>(LockedIO::new(expect_data.as_slice()), repeat(Duration::from_millis(10)));

        let mut actual_data = Vec::new();
        assert!(r.read_to_end(&mut actual_data).await.is_ok());
        assert_eq!(expect_data, actual_data);
    }

    #[tokio::test]
    async fn test_reconnect_write() {
        let expect_data = random_data();

        let locked = LockedIO::new(Vec::new());
        let mut w = connect::<RandomDisconnect<_>, _, _>(locked.clone(), repeat(Duration::from_millis(10)));
        assert!(w.write_all(&expect_data).await.is_ok());
        let actual_data = locked.inner.lock().unwrap().clone();
        assert_eq!(expect_data, actual_data);
    }

    fn random_data() -> Vec<u8> {
        let mut data = vec![0; 4698];
        rand::thread_rng().fill_bytes(&mut data);
        data
    }
}
