pub use memory::MemoryRunningStore;

use crate::{
    builtin::simple_processor::{RunningState, RunningStore, RunningTask},
    core::{Task, TaskId},
};

/// TODO: doc
pub mod memory {
    use std::{convert::Infallible, sync::Arc, time::Duration};

    use futures_util::future::BoxFuture;
    use indexmap::IndexMap;
    use tokio::{
        sync::{oneshot, Mutex},
        task::JoinHandle,
        time,
    };

    use super::*;

    /// TODO: doc
    pub struct NewMemoryRunningStore<S> {
        store: S,
        background_job: BoxFuture<'static, ()>,
    }

    impl<S> NewMemoryRunningStore<S> {
        /// TODO: doc
        pub fn spawn(self) -> S {
            tokio::spawn(self.background_job);
            self.store
        }
    }

    /// TODO: doc
    #[derive(Debug, Clone)]
    pub struct MemoryRunningStore<T: Task> {
        task_expire: Duration,
        inner: Arc<Mutex<IndexMap<TaskId, RunningTask<T>>>>,
    }

    impl<T: Task> MemoryRunningStore<T> {
        /// TODO: doc
        pub fn new(task_expire: Duration) -> NewMemoryRunningStore<Self> {
            let store = Self {
                task_expire,
                inner: Default::default(),
            };
            let inner = store.inner.clone();
            NewMemoryRunningStore {
                background_job: Box::pin(async move {
                    //TODO(0x5459): Duration::from_secs(5)
                    tokio::spawn(MemoryRunningStore::start_prune(inner, task_expire, Duration::from_secs(5)));
                }),
                store,
            }
        }

        async fn start_prune(
            inner: Arc<Mutex<IndexMap<TaskId, RunningTask<T>>>>,
            task_expire: Duration,
            interval: Duration,
        ) -> (JoinHandle<()>, oneshot::Sender<()>) {
            let (shutdown_tx, mut shutdown_rx) = oneshot::channel();
            let mut timer = time::interval(interval);

            let join_handle = tokio::spawn(async move {
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

                    let mut inner = inner.lock().await;
                    let should_prune: Vec<_> = inner
                        .iter()
                        .take(200)
                        .filter_map(|(task_id, task)| {
                            // TODO handle unwrap()
                            if task.started_at.elapsed().unwrap() >= task_expire {
                                Some(task_id.clone())
                            } else {
                                None
                            }
                        })
                        .collect();
                    should_prune.iter().for_each(|should_prune_task_id| {
                        inner.remove(should_prune_task_id);
                    });
                }
            });

            (join_handle, shutdown_tx)
        }
    }

    impl<T: Task> RunningStore<T> for MemoryRunningStore<T> {
        type Error = Infallible;
        type ListFut = BoxFuture<'static, Result<Vec<RunningTask<T>>, Self::Error>>;
        type InsertFut = BoxFuture<'static, Result<(), Self::Error>>;
        type UpdateStateFut = BoxFuture<'static, Result<(), Self::Error>>;

        fn list<'a>(&self, task_ids: Vec<TaskId>) -> Self::ListFut {
            let inner = self.inner.clone();
            Box::pin(async move {
                let inner = inner.lock().await;
                Ok(task_ids.iter().filter_map(|id| inner.get(id).cloned()).collect())
            })
        }

        fn insert(&self, running_task: RunningTask<T>) -> Self::InsertFut {
            let inner = self.inner.clone();
            Box::pin(async move {
                inner.lock().await.insert(running_task.task_id.clone(), running_task);
                Ok(())
            })
        }

        fn update_state(&self, task_id: &TaskId, state: RunningState<T>) -> Self::UpdateStateFut {
            let inner = self.inner.clone();
            let task_id = task_id.clone();
            Box::pin(async move {
                let task_id = task_id;
                let mut inner = inner.lock().await;
                // TODO: needs error if task_id not exists?
                if let Some(running_task) = inner.get_mut(&task_id) {
                    running_task.state = state;
                }
                Ok(())
            })
        }
    }
}
