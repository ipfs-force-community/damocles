use std::task::{Context, Poll};

use crate::{Processor, Task};

#[derive(Debug, Clone)]
pub struct TowerWrapper<P>(pub P);

impl<P, T> tower::Service<T> for TowerWrapper<P>
where
    T: Task,
    P: Processor<T>,
{
    type Response = T::Output;
    type Error = P::Error;
    type Future = P::Future;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.0.poll_ready(cx)
    }

    fn call(&mut self, task: T) -> Self::Future {
        self.0.process(task)
    }
}

impl<P, T> Processor<T> for TowerWrapper<P>
where
    T: Task,
    P: tower::Service<T, Response = T::Output>,
{
    type Error = P::Error;
    type Future = P::Future;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.0.poll_ready(cx)
    }

    fn process(&mut self, task: T) -> Self::Future {
        self.0.call(task)
    }
}

impl<P> tower::load::Load for TowerWrapper<P>
where
    P: tower::load::Load,
{
    type Metric = P::Metric;

    fn load(&self) -> Self::Metric {
        self.0.load()
    }
}
