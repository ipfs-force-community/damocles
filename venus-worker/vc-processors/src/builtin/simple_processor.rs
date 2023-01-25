use std::{
    collections::HashSet,
    future::Future,
    sync::Arc,
    task::{Context, Poll},
    time::{Duration, SystemTime},
};

use futures_util::future::BoxFuture;
use indexmap::IndexMap;
use tokio::{
    sync::{oneshot, Mutex},
    task::{spawn_blocking, JoinHandle},
    time,
};

use crate::core::{Processor, Task, TaskId, TaskIdExtractor};

use super::executor::TaskExecutor;

/// TODO: doc
pub mod running_store;

/// TODO: doc
pub struct NewSimpleProcessor<P> {
    simple_processor: P,
    background_job: BoxFuture<'static, ()>,
}

impl<P> NewSimpleProcessor<P> {
    /// TODO: doc
    pub fn spawn(self) -> P {
        tokio::spawn(self.background_job);
        self.simple_processor
    }
}

/// A simple Processor implementation
/// No task retry feature
pub struct SimpleProcessor<T: Task, IE, EX, RS> {
    task_id_extractor: IE,
    task_executor: EX,
    running: RS,
    waiting: Arc<Mutex<IndexMap<TaskId, oneshot::Sender<Result<T::Output, String>>>>>,
}

impl<T, IE, EX, RS> SimpleProcessor<T, IE, EX, RS>
where
    T: Task,
    RS: RunningStore<T> + Send + Sync + 'static,
    EX: Send + Sync + 'static,
    IE: Send + Sync + 'static,
{
    /// TODO: doc
    pub fn new(task_id_extractor: IE, task_executor: EX, running: RS) -> NewSimpleProcessor<Arc<Self>> {
        let p = Arc::new(SimpleProcessor {
            task_id_extractor,
            task_executor,
            running,
            waiting: Default::default(),
        });

        NewSimpleProcessor {
            simple_processor: p.clone(),
            background_job: Box::pin(async move {
                // TODO(0x5459): Duration::from_secs(5)
                tokio::spawn(Self::start_scan_waiting(p, Duration::from_secs(5)));
            }),
        }
    }

    async fn start_scan_waiting(self: Arc<Self>, interval: Duration) -> (JoinHandle<()>, oneshot::Sender<()>) {
        let (shutdown_tx, mut shutdown_rx) = oneshot::channel();
        let mut timer = time::interval(interval);

        let join_handler = tokio::spawn(async move {
            loop {
                tokio::select! {
                    // Wait for interval
                    _ = timer.tick() => {
                    },
                    _ = &mut shutdown_rx => {
                        tracing::info!("TODO");
                        return;
                    }
                }

                let waiting_task_ids: HashSet<_> = self.waiting.lock().await.keys().take(200).cloned().collect();
                if waiting_task_ids.is_empty() {
                    continue;
                }

                let running_tasks = self.running.list(waiting_task_ids.iter().cloned().collect()).await.unwrap();
                let running_task_ids: HashSet<_> = running_tasks.iter().map(|t| t.task_id.clone()).collect();

                let mut waiting_map = self.waiting.lock().await;

                // Remove non-running waiting channel
                for not_running_task_id in waiting_task_ids.difference(&running_task_ids) {
                    if let Some(sender) = waiting_map.remove(not_running_task_id) {
                        let _ = sender.send(Err(format!("task {} not running", not_running_task_id)));
                    }
                }

                for task in running_tasks {
                    match task.state {
                        RunningState::Finished { result, .. } => {
                            if let Some(sender) = waiting_map.remove(&task.task_id) {
                                let _ = sender.send(result);
                            }
                        }
                        RunningState::Running => {}
                    }
                }
            }
        });
        (join_handler, shutdown_tx)
    }
}

impl<T, IE, EX, RS> Processor<T> for Arc<SimpleProcessor<T, IE, EX, RS>>
where
    T: Task,
    IE: TaskIdExtractor<T> + Send + Sync + 'static,
    EX: TaskExecutor<T> + Send + Sync + 'static,
    RS: RunningStore<T> + Send + Sync + 'static,
{
    type Error = String;
    type WaitTaskFut = BoxFuture<'static, Result<T::Output, Self::Error>>;

    fn poll_ready(&self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn start_task(&self, task: T) -> Result<TaskId, Self::Error> {
        let task_id = self.task_id_extractor.extract(&task).unwrap();
        let task_id_cloned = task_id.clone();

        let this = self.clone();
        tokio::spawn(async move {
            // TODO: handle result
            if let Err(e) = this
                .running
                .insert(RunningTask {
                    task_id: task_id.clone(),
                    started_at: SystemTime::now(),
                    state: RunningState::Running,
                })
                .await
            {
                tracing::error!(error = ?e, "insert task into running store");
                return;
            }

            let this_cloned = this.clone();
            let task_result = match spawn_blocking(move || this_cloned.task_executor.exec(task)).await {
                Ok(task_result) => task_result.map_err(|e| e.to_string()),
                Err(join_error) => {
                    if join_error.is_panic() {
                        let panic_error = join_error.into_panic();
                        Err(match panic_error.downcast_ref::<&str>() {
                            Some(msg) => msg.to_string(),
                            None => panic_error
                                .downcast_ref::<String>()
                                .cloned()
                                .unwrap_or_else(|| "non string panic payload".to_string()),
                        })
                    } else {
                        Err(join_error.to_string())
                    }
                }
            };

            if let Err(e) = this
                .running
                .update_state(
                    &task_id,
                    RunningState::Finished {
                        result: task_result.clone(),
                        finished_at: SystemTime::now(),
                    },
                )
                .await
            {
                tracing::error!(error = ?e, "update task state");
            }

            if let Some(send_task_result) = this.waiting.lock().await.remove(&task_id) {
                let _ = send_task_result.send(task_result);
            }
        });
        Ok(task_id_cloned)
    }

    fn wait_task(&self, task_id: TaskId) -> Self::WaitTaskFut {
        let waiting = self.waiting.clone();

        Box::pin(async move {
            let (tx, rx) = oneshot::channel();
            let _ = waiting.lock().await.insert(task_id, tx);
            match rx.await {
                Ok(out) => out,
                Err(_recv_err) => Err("wait task channel closed.".to_string()),
            }
        })
    }
}

/// A trait defining the interface for a task state storage.
pub trait RunningStore<T: Task> {
    /// The error type used by this RunningStore.
    type Error: std::error::Error;

    /// Type of `get` future.
    type ListFut: Future<Output = Result<Vec<RunningTask<T>>, Self::Error>> + Send;
    /// Type of `insert` future.
    type InsertFut: Future<Output = Result<(), Self::Error>> + Send;
    /// Type of `update_state` future.
    type UpdateStateFut: Future<Output = Result<(), Self::Error>> + Send;

    /// TODO doc
    fn list(&self, task_ids: Vec<TaskId>) -> Self::ListFut;
    /// TODO doc
    fn insert(&self, running_task: RunningTask<T>) -> Self::InsertFut;
    /// TODO doc
    fn update_state(&self, task_id: &TaskId, state: RunningState<T>) -> Self::UpdateStateFut;
}

/// A running task
#[derive(Debug, Clone)]
pub struct RunningTask<T: Task> {
    task_id: TaskId,
    started_at: SystemTime,
    state: RunningState<T>,
}

/// TODO doc
#[derive(Debug, Clone)]
pub enum RunningState<T: Task> {
    /// Represents that the task is running,
    Running,
    /// Represents that the task is finished,
    Finished {
        /// Task output
        result: Result<T::Output, String>,
        /// The time when this task finished
        finished_at: SystemTime,
    },
}
