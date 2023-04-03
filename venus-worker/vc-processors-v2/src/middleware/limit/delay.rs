use std::{
    future::Future,
    pin::Pin,
    task::{ready, Context, Poll},
    time::Duration,
};

use pin_project::pin_project;
use tower::util::Oneshot;

use crate::Task;

/// A middleware which delays sending the request to the underlying service.
#[derive(Debug, Clone)]
pub struct Delay<S> {
    service: S,
    delay: Duration,
}

impl<S> Delay<S> {
    /// Create a new Delay.
    pub fn new(service: S, delay: Duration) -> Self {
        Self { service, delay }
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

impl<Tsk, S> tower::Service<Tsk> for Delay<S>
where
    Tsk: Task,
    S: tower::Service<Tsk> + Clone,
{
    type Error = S::Error;
    type Response = S::Response;
    type Future = DelayStartFuture<Tsk, S>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        // Calling self.service.poll_ready would reserve a slot for the delayed request,
        // potentially well in advance of actually making it. Instead, signal readiness here and
        // treat the service as a StartOnce in the future.
        self.service.poll_ready(cx)
    }

    fn call(&mut self, task: Tsk) -> Self::Future {
        DelayStartFuture {
            service: Some(self.service.clone()),
            state: State::Delaying {
                delay: tokio::time::sleep(self.delay),
                task: Some(task),
            },
        }
    }
}

#[pin_project(project=StateProj)]
#[derive(Debug)]
enum State<Tsk, F> {
    Delaying {
        #[pin]
        delay: tokio::time::Sleep,
        task: Option<Tsk>,
    },
    Started {
        #[pin]
        fut: F,
    },
}

/// The Future type of Delay
#[pin_project]
pub struct DelayStartFuture<Tsk, S>
where
    Tsk: Task,
    S: tower::Service<Tsk>,
{
    service: Option<S>,
    #[pin]
    state: State<Tsk, Oneshot<S, Tsk>>,
}

impl<Tsk, S> Future for DelayStartFuture<Tsk, S>
where
    Tsk: Task,
    S: tower::Service<Tsk>,
{
    type Output = <S::Future as Future>::Output;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let mut pinned = self.project();

        loop {
            match pinned.state.as_mut().project() {
                StateProj::Delaying { delay, task } => {
                    ready!(delay.poll(cx));
                    let task = task.take().expect("Missing task in delay");
                    let proc = pinned.service.take().expect("Missing processor in delay");
                    pinned.state.set(State::Started {
                        fut: Oneshot::new(proc, task),
                    });
                }
                StateProj::Started { fut } => {
                    return fut.poll(cx);
                }
            }
        }
    }
}

impl<S> tower::load::Load for Delay<S>
where
    S: tower::load::Load,
{
    type Metric = S::Metric;
    fn load(&self) -> Self::Metric {
        self.service.load()
    }
}

/// A middleware which delays sending the request to the underlying service.
#[derive(Debug, Clone)]
pub struct DelayLayer {
    delay: Duration,
}

impl DelayLayer {
    /// Creates a new [`DelayLayer`] with the provided `delay`.
    pub fn new(delay: Duration) -> Self {
        Self { delay }
    }
}

impl<S> tower::Layer<S> for DelayLayer {
    type Service = Delay<S>;

    fn layer(&self, inner: S) -> Self::Service {
        Delay::new(inner, self.delay)
    }
}
