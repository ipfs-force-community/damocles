use std::{
    future::{poll_fn, Future},
    marker::PhantomData,
};

use crate::{util::tower::TowerWrapper, Processor, Task};

/// A wrapper type of crate::Processor that provides some useful methods for processing tasks.
#[derive(Debug, Clone)]
pub struct ProcessorClient<Tsk, S> {
    processor: S,
    _pd: PhantomData<Tsk>,
}

impl<Tsk, S> ProcessorClient<Tsk, S>
where
    Tsk: Task,
    S: tower::Service<Tsk, Response = Tsk::Output>,
{
    /// Creates a new `ProcessorClient` with given `svc`
    pub fn new(svc: S) -> Self {
        Self {
            processor: svc,
            _pd: PhantomData,
        }
    }

    /// Call the inner process
    pub async fn process(&mut self, task: Tsk) -> <S::Future as Future>::Output {
        self.ready().await?;
        self.processor.call(task).await
    }

    async fn ready(&mut self) -> Result<(), S::Error> {
        poll_fn(|cx| self.processor.poll_ready(cx)).await
    }
}

impl<Tsk, P> ProcessorClient<Tsk, TowerWrapper<P>>
where
    Tsk: Task,
    P: Processor<Tsk>,
{
    /// Creates a new Processor with given processor
    pub fn from_processor(proc: P) -> Self {
        Self::new(TowerWrapper(proc))
    }
}
