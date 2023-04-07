//!  provides types for producer
//!

use std::{
    fmt,
    future::Future,
    sync::Arc,
    task::{Context, Poll},
    time::Duration,
};

use futures_util::{future::BoxFuture, Sink, Stream};
use serde::{de::DeserializeOwned, Serialize};
use tokio::{select, sync::mpsc::unbounded_channel, time::sleep};

use crate::{Processor, Request, Response, Task};

pub use self::client::Error;
use self::{client::ConsumerClient, dispatch::Dispatch};

mod client;
mod dispatch;

mod inflight_requests;

/// A producer and dispatch pair. The dispatch drives the sending and receiving of requests
/// and must be polled continuously or spawned.
pub struct NewProducer<P, D> {
    producer: P,
    dispatch: D,
}

impl<P, D> NewProducer<P, D>
where
    D: Future + Send + 'static,
    D::Output: Send,
{
    /// Spawn the dispatch and return the producer
    pub fn spawn(self) -> P {
        tokio::spawn(self.dispatch);
        self.producer
    }
}

/// Producer is the client that communicates with the external processor
#[derive(Debug, Clone)]
pub struct Producer<Tsk: Task, TskId> {
    client: Arc<ConsumerClient<Tsk, TskId>>,
}

impl<Tsk, TskId> Producer<Tsk, TskId>
where
    Tsk: Task,
    TskId: Serialize + DeserializeOwned + Clone,
{
    /// Create a producer with given `transport`.
    /// The `transport` is the communication channel with the external processor
    pub fn new<TP, E>(transport: TP) -> NewProducer<Self, Dispatch<TP, Tsk, TskId>>
    where
        TP: Sink<Request<Tsk, TskId>, Error = E> + Stream<Item = Result<Response<Tsk::Output, TskId>, E>> + Unpin,
    {
        let (to_dispatch_cancellation, pending_cancellations) = unbounded_channel();
        let (to_dispatch_request, pending_requests) = unbounded_channel();

        let producer = Self {
            client: Arc::new(ConsumerClient::new(to_dispatch_request, to_dispatch_cancellation)),
        };

        NewProducer {
            producer,
            dispatch: Dispatch::new(transport, pending_cancellations, pending_requests),
        }
    }

    async fn process(client: Arc<ConsumerClient<Tsk, TskId>>, task: Tsk) -> Result<Tsk::Output, Error>
    where
        TskId: fmt::Display,
    {
        loop {
            let task_id: TskId = client.start_task(task.clone()).await?;
            tracing::debug!(task_id = tracing::field::display(&task_id), "task was started");
            loop {
                tracing::trace!(task_id = tracing::field::display(&task_id), "wait task");

                let wait_interval = sleep(Duration::from_secs(10));
                tokio::pin!(wait_interval);
                select! {
                    _ = &mut wait_interval => {
                        tracing::trace!(task_id = tracing::field::display(&task_id), "wait_task timed out");
                        continue;
                    },
                    result_option = client.wait_task(task_id.clone()) => {
                        match result_option {
                            Ok(None) => {
                                tracing::debug!(task_id = tracing::field::display(&task_id), "the task id does not exist in the consumer, retry sending the task");
                                break
                            }
                            Ok(Some(output)) => return Ok(output),
                            Err(e) => return Err(e)
                        }
                    },
                }
            }
        }
    }
}

impl<Tsk, TskId> Processor<Tsk> for Producer<Tsk, TskId>
where
    Tsk: Task,
    TskId: fmt::Display + Serialize + DeserializeOwned + Clone + Send + 'static,
{
    type Error = Error;
    type Future = BoxFuture<'static, Result<Tsk::Output, Self::Error>>;

    fn poll_ready(&mut self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn process(&mut self, task: Tsk) -> Self::Future {
        Box::pin(Producer::<Tsk, TskId>::process(self.client.clone(), task))
    }
}
