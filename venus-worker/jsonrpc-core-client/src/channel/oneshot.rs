use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};

use async_std::{
    channel::{bounded, Receiver, Sender},
    stream::Stream,
};

pub use async_std::channel::RecvError;

pub fn oneshot<T>() -> (Tx<T>, Rx<T>) {
    let (tx, rx) = bounded(1);
    (Tx(tx), Rx(Some(rx)))
}

pub struct Tx<T>(Sender<T>);

impl<T> Tx<T> {
    pub fn send(self, msg: T) -> Result<(), T> {
        self.0.try_send(msg).map_err(|e| e.into_inner())
    }
}

pub struct Rx<T>(Option<Receiver<T>>);

impl<T> Future for Rx<T> {
    type Output = Result<T, RecvError>;

    fn poll(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let rx = match self.0.as_mut() {
            Some(rx) => rx,
            None => return Poll::Ready(Err(RecvError)),
        };

        match Pin::new(rx).poll_next(cx) {
            Poll::Ready(opt) => {
                let output = opt.ok_or(RecvError);
                self.0.take().map(drop);
                Poll::Ready(output)
            }

            Poll::Pending => Poll::Pending,
        }
    }
}
