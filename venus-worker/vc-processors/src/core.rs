//! This module provides the most important types and abstractions
//!

use std::{
    error::Error as StdError,
    fmt::Debug,
    future::Future,
    task::{ready, Context, Poll},
};

use futures_util::{
    future::{ready, BoxFuture, Either, MapOk, Ready},
    TryFutureExt,
};
use serde::{de::DeserializeOwned, Deserialize, Serialize};

/// Task of a specific stage, with Output & Error defined
pub trait Task
where
    Self: Debug + Clone + Send + Sync + 'static,
    Self::Output: Serialize + DeserializeOwned + Clone + Debug + Send + Sync + 'static,
{
    /// The stage name.
    const STAGE: &'static str;

    /// The output type
    type Output;
}

/// Represents task id
pub type TaskId = String;

/// Processor of a specific task type
pub trait Processor<T: Task> {
    /// TODO: doc
    type Error: Into<Box<dyn StdError + Send + Sync>> + Send + Debug;

    /// Type of wait_task future.
    type WaitTaskFut: Future<Output = Result<<T as Task>::Output, Self::Error>> + Send;

    /// TODO: doc
    fn poll_ready(&self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>>;

    /// Starts a task and returns a unique id of this task.
    fn start_task(&self, task: T) -> Result<TaskId, Self::Error>;

    /// Waiting for a particular task to be finished.
    fn wait_task(&self, task_id: TaskId) -> Self::WaitTaskFut;
}

/// TODO: doc
pub trait DynProcessor<T: Task> {
    /// TODO: doc
    fn poll_ready(&self, cx: &mut Context<'_>) -> Poll<Result<(), Box<dyn StdError + Send + Sync>>>;
    /// TODO: doc
    fn start_task(&self, task: T) -> Result<TaskId, Box<dyn StdError + Send + Sync>>;
    /// TODO: doc
    fn wait_task(&self, task_id: TaskId) -> BoxFuture<Result<<T as Task>::Output, Box<dyn StdError + Send + Sync>>>;
}

impl<T: Task, P: Processor<T> + Sync> DynProcessor<T> for P {
    fn poll_ready(&self, cx: &mut Context<'_>) -> Poll<Result<(), Box<dyn StdError + Send + Sync>>> {
        Processor::poll_ready(self, cx).map_err(Into::into)
    }

    fn start_task(&self, task: T) -> Result<TaskId, Box<dyn StdError + Send + Sync>> {
        Processor::start_task(self, task).map_err(Into::into)
    }

    fn wait_task(&self, task_id: TaskId) -> BoxFuture<Result<<T as Task>::Output, Box<dyn StdError + Send + Sync>>> {
        Box::pin(async move { Processor::wait_task(self, task_id).await.map_err(Into::into) })
    }
}

/// Task id extractor
pub trait TaskIdExtractor<T> {
    /// TODO: doc
    type Error: Debug;

    /// Extract task id from task
    fn extract(&self, task: &T) -> Result<TaskId, Self::Error>;
}

impl<T, F, E> TaskIdExtractor<T> for F
where
    F: Fn(&T) -> Result<TaskId, E>,
    E: Debug,
{
    type Error = E;

    fn extract(&self, task: &T) -> Result<TaskId, Self::Error> {
        self(task)
    }
}

/// vc-processor ipc request
#[derive(Debug, Serialize, Deserialize)]
pub enum Request<T> {
    /// request for start_task method
    StartTask(T),
    /// request for wait_task method
    WaitTask(TaskId),
}

/// vc-processor ipc response
#[derive(Debug, Serialize, Deserialize)]
pub enum Response<T: Task> {
    /// response for start_task method
    StartTask(TaskId),
    /// response for start_task method
    WaitTask(<T as Task>::Output),
}

/// A wrapper type to convert the Processor to tower_service::Service
#[derive(Clone)]
pub struct TowerServiceWrapper<P>(pub P);

impl<T, P> tower_service::Service<Request<T>> for TowerServiceWrapper<P>
where
    T: Task,
    P: Processor<T>,
{
    type Response = Response<T>;

    type Error = P::Error;

    type Future =
        Either<MapOk<<P as Processor<T>>::WaitTaskFut, fn(<T as Task>::Output) -> Response<T>>, Ready<Result<Response<T>, Self::Error>>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.0.poll_ready(cx)
    }

    fn call(&mut self, req: Request<T>) -> Self::Future {
        match req {
            Request::WaitTask(task_id) => Either::Left(self.0.wait_task(task_id).map_ok(Response::WaitTask)),
            Request::StartTask(task) => Either::Right(ready(self.0.start_task(task).map(Response::StartTask))),
        }
    }
}

/// TODO: DOC
pub struct ProcessorClient<T: Task> {
    ipc_client: vc_ipc::client::Client<Request<T>, Response<T>>,
}

impl<T: Task> ProcessorClient<T> {
    /// TODO: DOC
    pub fn new(ipc_client: vc_ipc::client::Client<Request<T>, Response<T>>) -> Self {
        Self { ipc_client }
    }

    /// Starts a task and returns a unique id of this task.
    pub async fn start_task(&self, task: T) -> Result<TaskId, vc_ipc::client::IpcError> {
        match self.ipc_client.call(Request::StartTask(task)).await? {
            Response::StartTask(task_id) => Ok(task_id),
            Response::WaitTask(_) => unreachable!(),
        }
    }

    /// Waiting for a particular task to be finished.
    pub async fn wait_task(&self, task_id: &TaskId) -> Result<T::Output, vc_ipc::client::IpcError> {
        match self.ipc_client.call(Request::WaitTask(task_id.clone())).await? {
            Response::StartTask(_) => unreachable!(),
            Response::WaitTask(x) => Ok(x),
        }
    }
}
