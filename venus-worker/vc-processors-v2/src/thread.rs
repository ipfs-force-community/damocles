use std::{
    io,
    marker::PhantomData,
    task::{Context, Poll},
};

use futures_util::{future::Map, FutureExt};
use tokio::task::{spawn_blocking, JoinError, JoinHandle};

use crate::{consumer::Executor, ConsumerError, Processor, Task};

/// ThreadProcessor is used to start threads directly in the current process to execute tasks.
#[derive(Debug)]
pub struct ThreadProcessor<Tsk: Task, Exe: Executor<Tsk>> {
    executor: Exe,
    _pd: PhantomData<Tsk>,
}

impl<Tsk: Task, Exe: Executor<Tsk>> ThreadProcessor<Tsk, Exe> {
    /// Create a new ThreadProcessor with given `executor`
    pub fn new(executor: Exe) -> Self {
        Self {
            executor,
            _pd: PhantomData,
        }
    }
}

impl<Tsk: Task, Exe: Executor<Tsk>> Clone for ThreadProcessor<Tsk, Exe> {
    fn clone(&self) -> Self {
        Self {
            executor: self.executor.clone(),
            _pd: PhantomData,
        }
    }
}

impl<Tsk, Exe> Processor<Tsk> for ThreadProcessor<Tsk, Exe>
where
    Tsk: Task,
    Exe: Executor<Tsk> + Send + Clone + 'static,
    Exe::Error: Into<ConsumerError> + Send,
{
    type Error = crate::ProducerError;

    type Future = Map<
        JoinHandle<Result<Tsk::Output, Exe::Error>>,
        fn(Result<Result<Tsk::Output, Exe::Error>, JoinError>) -> Result<Tsk::Output, crate::ProducerError>,
    >;

    fn poll_ready(&mut self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn process(&mut self, task: Tsk) -> Self::Future {
        let executor = self.executor.clone();
        spawn_blocking(move || executor.execute(task)).map(|res| match res {
            Ok(Ok(output)) => Ok(output),
            Ok(Err(e)) => Err(crate::ProducerError::Consumer(e.into())),
            Err(je) => Err(ConsumerError {
                kind: io::ErrorKind::Other,
                detail: je.to_string(),
            }
            .into()),
        })
    }
}
