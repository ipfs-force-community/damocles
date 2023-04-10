#![deny(missing_docs)]
//! vc-processors contains the types and builtin external processors for
//! [venus-cluster/venus-worker](https://github.com/ipfs-force-community/venus-cluster/tree/main/venus-worker).
//!
//! This crate aims at providing an easy-to-use, buttery-included framework for third-party
//! developers to implement customized external processors.
//!
//! This crate could be used to:
//! 1. interact with builtin external processors in venus-worker.
//! 2. wrap close-source processors for builtin tasks in venus-worker.
//! 3. implement any customized external processors for other usecases.
//!
//! The [examples](https://github.com/ipfs-force-community/venus-cluster/tree/main/venus-worker/vc-processors/examples) show more details about the usages.
//!

// #[cfg(any(feature = "builtin-tasks", feature = "builtin-processors"))]
// pub mod builtin;

use std::{
    fmt::Debug,
    future::Future,
    task::{Context, Poll},
};

pub use self::client::ProcessorClient;
pub use self::producer::Error as ProducerError;
pub use self::request::Request;
pub use self::response::{ConsumerError, Response};
pub use self::thread::ThreadProcessor;
use serde::{de::DeserializeOwned, Serialize};

mod client;
pub mod consumer;
mod context;
pub mod middleware;
pub mod producer;
mod request;
mod response;
pub mod sys;
mod thread;
#[allow(missing_docs)]
pub mod transport;
#[allow(missing_docs)]
pub mod util;

/// Task of a specific stage, with Output & Error defined
pub trait Task
where
    Self: Serialize + DeserializeOwned + Clone + Debug + Send + Sync + Unpin + 'static,
    Self::Output: Serialize + DeserializeOwned + Debug + Send + Sync + Unpin + 'static,
{
    /// The stage name.
    const STAGE: &'static str;

    /// The output type
    type Output;
}

/// Processor of a specific task type
pub trait Processor<Tsk: Task> {
    /// Errors produced by the Processor.
    type Error;
    /// The future response value.
    type Future: Future<Output = Result<Tsk::Output, Self::Error>>;

    /// Attempts to prepare the Processor to receive a task.
    /// This method must be called and return `Poll::Ready(Ok(()))` prior to each call to `process`.
    ///
    /// This method returns `Poll::Ready` once the underlying processor is ready to receive task.
    /// If this method returns `Poll::Pending`, the current task is registered to be notified (via `cx.waker().wake_by_ref()`) when `poll_ready` should be called again.
    ///
    /// In most cases, if the processor encounters an error, the processor will permanently be unable to receive tasks.
    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>>;

    /// Process the request and return the response asynchronously.
    /// Before dispatching a request, poll_ready must be called and return Poll::Ready(Ok(())).
    fn process(&mut self, task: Tsk) -> Self::Future;
}

/// Consumer is the trait that an external processor should implement.
/// Used to execute tasks passed in by the producer
pub trait Consumer<Tsk: Task> {
    /// the type of task id
    /// task id is generated by the consumer and returned to the producer
    type TaskId: Serialize + DeserializeOwned;
    /// Errors produced by the Consumer.
    type Error;
    /// The future type of `start_task`.
    type StartTaskFut: Future<Output = Result<Self::TaskId, Self::Error>>;
    /// The future type of `wait_task`.
    type WaitTaskFut: Future<Output = Result<Option<Tsk::Output>, Self::Error>>;

    /// Asynchronously starts executing a task and returns a task id.
    /// Generally, the task id of each task should be unique.
    fn start_task(&mut self, task: Tsk) -> Self::StartTaskFut;
    /// Waits for the task corresponding to the given task id to complete.
    /// Returns None to indicate that the task corresponding to the given task id has not been executed (start_task has not been called).
    fn wait_task(&mut self, task_id: Self::TaskId) -> Self::WaitTaskFut;
}

impl<'a, P, Tsk: Task> Processor<Tsk> for &'a mut P
where
    P: Processor<Tsk> + 'a,
{
    type Error = P::Error;
    type Future = P::Future;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), P::Error>> {
        (*self).poll_ready(cx)
    }

    fn process(&mut self, task: Tsk) -> Self::Future {
        (*self).process(task)
    }
}

impl<P, Tsk: Task> Processor<Tsk> for Box<P>
where
    P: Processor<Tsk> + ?Sized,
{
    type Error = P::Error;
    type Future = P::Future;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), P::Error>> {
        (**self).poll_ready(cx)
    }

    fn process(&mut self, task: Tsk) -> Self::Future {
        (**self).process(task)
    }
}

#[allow(missing_docs)]
#[inline]
pub fn ready_msg<T: Task>() -> String {
    ready_msg_by_name(T::STAGE)
}

#[allow(missing_docs)]
#[inline]
pub fn ready_msg_by_name(name: &str) -> String {
    format!("{} processor ready", name)
}
