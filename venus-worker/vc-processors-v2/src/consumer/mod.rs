//!  provides types or functions for consumer
//!

use std::{fmt, hash::Hash, num::NonZeroUsize, sync::Arc};

use futures_util::{future::BoxFuture, Sink, Stream};
use tokio::{
    sync::{oneshot, Mutex},
    task::spawn_blocking,
};

use crate::{Consumer, Request, Response, Task};

use self::{
    dispatch::Dispatch,
    task_id::{Generator as TaskIdGenerator, XXHash},
};

mod dispatch;
mod routing;
mod task_id;

/// Listen to the request passed in by transport and hand it over to the consumer for processing,
/// then write the result back to transport.
pub fn serve<Tsk, TP, C, E>(transport: TP, consumer: C) -> Dispatch<Tsk, C, TP>
where
    Tsk: Task,
    C: Consumer<Tsk>,
    TP: Sink<Response<Tsk::Output, C::TaskId>, Error = E> + Stream<Item = Result<Request<Tsk, C::TaskId>, E>>,
{
    Dispatch::new(consumer, transport)
}

/// An executor of tasks.
pub trait Executor<T: Task>: Clone {
    /// Errors produced by the Processor.
    type Error;

    /// Actually execute the given task and return the execution result
    fn execute(&self, task: T) -> Result<T::Output, Self::Error>;
}

/// DefaultConsumer implements the `crate::Consumer` trait and uses an LRU memory map internally
/// to store task execution status data
#[derive(Clone)]
pub struct DefaultConsumer<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk>,
    TaskIdGen: TaskIdGenerator<Tsk>,
{
    inner: DefaultConsumerInner<Tsk, Exe, TaskIdGen>,
    _cg: Arc<crate::sys::cgroup::CtrlGroup>,
}

impl<Tsk, Exe, TaskIdGen> DefaultConsumer<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk>,
    TaskIdGen: TaskIdGenerator<Tsk>,
    TaskIdGen::TaskId: Hash + Eq,
{
    /// Creates a new `DefaultConsumer` with given executor and `task_id_gen`
    pub fn new(executor: Exe, task_id_gen: TaskIdGen) -> Self {
        #[cfg(feature = "numa")]
        crate::sys::numa::try_set_preferred_from_env();

        Self {
            inner: DefaultConsumerInner {
                executor,
                task_id_gen,
                tasks: Arc::new(Mutex::new(lru::LruCache::new(unsafe { NonZeroUsize::new_unchecked(64) }))),
            },
            _cg: Arc::new(crate::sys::cgroup::try_set_from_env()),
        }
    }
}

impl<Tsk, Exe> DefaultConsumer<Tsk, Exe, XXHash>
where
    Tsk: Task,
    Exe: Executor<Tsk>,
{
    /// Creates a new `DefaultConsumer` with given executor thats use xxhash as task id generator
    pub fn new_xxhash(executor: Exe) -> Self {
        Self::new(executor, XXHash)
    }
}

impl<Tsk, Exe, TaskIdGen> Consumer<Tsk> for DefaultConsumer<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk> + Send + 'static,
    Exe::Error: Send,
    TaskIdGen: TaskIdGenerator<Tsk> + Send + 'static,
    TaskIdGen::TaskId: fmt::Debug + Hash + Eq + Send + 'static,
{
    type TaskId = TaskIdGen::TaskId;
    type Error = Exe::Error;

    type StartTaskFut = BoxFuture<'static, Result<TaskIdGen::TaskId, Self::Error>>;
    type WaitTaskFut = BoxFuture<'static, Result<Option<Tsk::Output>, Self::Error>>;

    fn start_task(&mut self, task: Tsk) -> Self::StartTaskFut {
        Box::pin(self.inner.clone().start_task(task))
    }

    fn wait_task(&mut self, task_id: Self::TaskId) -> Self::WaitTaskFut {
        Box::pin(self.inner.clone().wait_task(task_id))
    }
}

struct DefaultConsumerInner<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk>,
    TaskIdGen: TaskIdGenerator<Tsk>,
{
    executor: Exe,
    task_id_gen: TaskIdGen,
    #[allow(clippy::type_complexity)]
    tasks: Arc<Mutex<lru::LruCache<TaskIdGen::TaskId, TaskState<Tsk::Output, Exe::Error>>>>,
}

impl<Tsk, Exe, TaskIdGen> DefaultConsumerInner<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk> + Send + 'static,
    Exe::Error: Send,
    TaskIdGen: TaskIdGenerator<Tsk>,
    TaskIdGen::TaskId: fmt::Debug + Hash + Eq + Send + 'static,
{
    async fn start_task(self, task: Tsk) -> Result<TaskIdGen::TaskId, Exe::Error> {
        // TODO: try use spawn_blocking. but it require 'static lifetime
        let task_id = self.task_id_gen.generate(&task);

        let mut tasks = self.tasks.lock().await;
        if tasks.contains(&task_id) {
            return Ok(task_id);
        }

        tasks.put(task_id.clone(), TaskState::Running(TaskStateWatchers::new()));
        drop(tasks);

        let executor = self.executor.clone();
        let tasks = self.tasks.clone();
        let task_id_cloned = task_id.clone();
        spawn_blocking(move || {
            let result = executor.execute(task);
            let mut tasks = tasks.blocking_lock();
            if let Some(task_state) = tasks.get_mut(&task_id_cloned) {
                task_state.complete(result);
            } else {
                tracing::warn!(task_id = ?task_id_cloned, "the task id should exist in tasks");
                tasks.put(task_id_cloned, TaskState::Complete(result));
            }
        });
        Ok(task_id)
    }

    async fn wait_task(self, task_id: TaskIdGen::TaskId) -> Result<Option<Tsk::Output>, Exe::Error> {
        let tasks = self.tasks.clone();

        let mut tasks = tasks.lock().await;
        let watcher = match tasks.get_mut(&task_id) {
            Some(TaskState::Running(watchers)) => watchers.register_watcher(),
            Some(TaskState::Complete(_)) => match tasks.pop(&task_id) {
                Some(TaskState::Complete(result)) => return result.map(Some),
                _ => unreachable!(),
            },
            None => return Ok(None),
        };
        drop(tasks);

        // wait task done
        let _ = watcher.await;

        let mut tasks = self.tasks.lock().await;
        if let Some(TaskState::Complete(_)) = tasks.peek(&task_id) {
            if let Some(TaskState::Complete(result)) = tasks.pop(&task_id) {
                return result.map(Some);
            }
        }
        Ok(None)
    }
}

impl<Tsk, Exe, TaskIdGen> Clone for DefaultConsumerInner<Tsk, Exe, TaskIdGen>
where
    Tsk: Task,
    Exe: Executor<Tsk> + Clone,
    TaskIdGen: TaskIdGenerator<Tsk> + Clone,
{
    fn clone(&self) -> Self {
        Self {
            executor: self.executor.clone(),
            task_id_gen: self.task_id_gen.clone(),
            tasks: self.tasks.clone(),
        }
    }
}

enum TaskState<Output, E> {
    Running(TaskStateWatchers),
    Complete(Result<Output, E>),
}

impl<Output, E> TaskState<Output, E> {
    pub fn complete(&mut self, result: Result<Output, E>) {
        match std::mem::replace(self, TaskState::Complete(result)) {
            TaskState::Running(watchers) => {
                watchers.notify_all();
            }
            TaskState::Complete(_) => {}
        }
    }
}

struct TaskStateWatchers(Vec<oneshot::Sender<()>>);

impl TaskStateWatchers {
    pub fn new() -> Self {
        Self(Vec::new())
    }

    pub fn register_watcher(&mut self) -> oneshot::Receiver<()> {
        let (task_state_tx, task_state_rx) = oneshot::channel();
        self.0.push(task_state_tx);
        task_state_rx
    }

    pub fn notify_all(self) {
        for tx in self.0 {
            let _ = tx.send(());
        }
    }
}
