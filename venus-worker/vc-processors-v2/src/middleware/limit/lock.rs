use std::{
    future::Future,
    pin::Pin,
    sync::Arc,
    task::{ready, Context, Poll},
};

use pin_project::pin_project;
use tokio::sync::{OwnedSemaphorePermit, Semaphore};
use tokio_util::sync::PollSemaphore;

/// A middleware that limits requests based on the given Semaphores
#[derive(Debug)]
pub struct Lock<S> {
    service: S,
    locks: Vec<PollSemaphore>,
    permits: Option<Vec<OwnedSemaphorePermit>>,
}

impl<S> Lock<S> {
    /// Create a new Lock.
    pub fn new(service: S, locks: Vec<Arc<Semaphore>>) -> Self {
        Self {
            service,
            locks: locks.into_iter().map(PollSemaphore::new).collect(),
            permits: None,
        }
    }

    /// Get a reference to the inner service
    pub fn get_ref(&self) -> &S {
        &self.service
    }

    /// Get a mutable reference to the inner service
    pub fn get_mut(&mut self) -> &mut S {
        &mut self.service
    }

    /// Consume `self`, returning the inner service
    pub fn into_inner(self) -> S {
        self.service
    }
}

impl<S: Clone> Clone for Lock<S> {
    fn clone(&self) -> Self {
        Self {
            service: self.service.clone(),
            locks: self.locks.clone(),
            permits: None,
        }
    }
}

impl<Tsk, S> tower::Service<Tsk> for Lock<S>
where
    S: tower::Service<Tsk>,
{
    type Error = S::Error;
    type Response = S::Response;
    type Future = ResponseFuture<S::Future>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        let permits = match self.permits.take() {
            Some(permits) => permits,
            None => {
                tracing::debug!("try acquire {} locks", self.locks.len());
                let mut permits = Vec::with_capacity(self.locks.len());
                for (i, lock) in self.locks.iter_mut().enumerate() {
                    match lock.poll_acquire(cx) {
                        Poll::Ready(Some(permit)) => {
                            permits.push(permit);
                        }
                        Poll::Ready(None) => {
                            tracing::error!("{}th lock closed", i + 1);
                        }
                        Poll::Pending => {
                            tracing::debug!("failed to acquire the {}th lock", i + 1);
                            return Poll::Pending;
                        }
                    }
                }
                permits
            }
        };
        tracing::debug!("all locks have been acquired");
        match self.service.poll_ready(cx) {
            Poll::Ready(Ok(())) => {
                self.permits = Some(permits);
                Poll::Ready(Ok(()))
            }
            x => x,
        }
    }

    fn call(&mut self, task: Tsk) -> Self::Future {
        let permits = self.permits.take().expect("poll_ready must be called first");
        let future = self.service.call(task);
        ResponseFuture::new(future, permits)
    }
}

/// Future for the [`Lock`] service.
///
/// [`Lock`]: crate::middleware::limit::Lock
#[derive(Debug)]
#[pin_project]
pub struct ResponseFuture<T> {
    #[pin]
    inner: T,
    // Keep this around so that it is dropped when the future completes
    _permits: Vec<OwnedSemaphorePermit>,
}

impl<T> ResponseFuture<T> {
    pub(crate) fn new(inner: T, _permits: Vec<OwnedSemaphorePermit>) -> ResponseFuture<T> {
        ResponseFuture { inner, _permits }
    }
}

impl<F, T, E> Future for ResponseFuture<F>
where
    F: Future<Output = Result<T, E>>,
{
    type Output = Result<T, E>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        Poll::Ready(ready!(self.project().inner.poll(cx)))
    }
}

impl<S> tower::load::Load for Lock<S>
where
    S: tower::load::Load,
{
    type Metric = S::Metric;
    fn load(&self) -> Self::Metric {
        self.service.load()
    }
}

/// A middleware that limits requests based on the given Semaphores
#[derive(Debug, Clone)]
pub struct LockLayer {
    locks: Vec<Arc<Semaphore>>,
}

impl LockLayer {
    /// Creates a new [`LockLayer`] with the provided `locks`.
    pub fn new(locks: Vec<Arc<Semaphore>>) -> Self {
        Self { locks }
    }
}

impl<S> tower::Layer<S> for LockLayer {
    type Service = Lock<S>;

    fn layer(&self, inner: S) -> Self::Service {
        Lock::new(inner, self.locks.clone())
    }
}
