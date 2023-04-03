use std::{
    future::Future,
    marker::PhantomData,
    pin::Pin,
    task::{ready, Context, Poll},
};

use pin_project::pin_project;

use crate::{
    request::RequestBody,
    response::{ConsumerError, ResponseBody},
    Consumer, Request, Response, Task,
};

impl<T: ?Sized, Tsk: Task> Routing<Tsk> for T where T: Consumer<Tsk> {}

pub trait Routing<Tsk: Task>: Consumer<Tsk> {
    fn call(&mut self, request: Request<Tsk, Self::TaskId>) -> CallFuture<'_, Tsk, Self>
    where
        Self: Sized,
    {
        CallFuture {
            consumer: self,
            state: CallState::Init(Some(request)),
            _pd: PhantomData,
        }
    }
}

#[pin_project]
pub struct CallFuture<'a, Tsk, C>
where
    Tsk: Task,
    C: Consumer<Tsk>,
{
    #[pin]
    consumer: &'a mut C,
    #[pin]
    state: CallState<Tsk, C::TaskId, C::StartTaskFut, C::WaitTaskFut>,
    _pd: PhantomData<Tsk>,
}

#[pin_project(project=CallStateProj)]
enum CallState<Tsk, TskId, F1, F2> {
    Init(Option<Request<Tsk, TskId>>),
    StartTask {
        request_id: u64,
        #[pin]
        fut: F1,
    },
    WaitTask {
        request_id: u64,
        #[pin]
        fut: F2,
    },
}

impl<Tsk, C> Future for CallFuture<'_, Tsk, C>
where
    Tsk: Task,
    C: Consumer<Tsk>,
    C::Error: Into<ConsumerError>,
{
    type Output = Option<Response<Tsk::Output, C::TaskId>>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let mut pinned = self.project();
        loop {
            match pinned.state.as_mut().project() {
                CallStateProj::Init(request) => {
                    let request = request.take().expect("illegal state");
                    let request_id = request.request_id;

                    pinned.state.set(match request.body {
                        RequestBody::StartTask { task } => CallState::StartTask {
                            request_id,
                            fut: pinned.consumer.start_task(task),
                        },
                        RequestBody::WaitTask { task_id } => CallState::WaitTask {
                            request_id,
                            fut: pinned.consumer.wait_task(task_id),
                        },
                        RequestBody::CancelRequest => {
                            // TODO: support cancel request,
                            return Poll::Ready(None);
                        }
                    });
                }
                CallStateProj::StartTask { request_id, fut } => {
                    return Poll::Ready(Some(Response::new(
                        *request_id,
                        ready!(fut.poll(cx)).map(ResponseBody::start_task).map_err(Into::into),
                    )));
                }
                CallStateProj::WaitTask { request_id, fut } => {
                    return Poll::Ready(Some(Response::new(
                        *request_id,
                        ready!(fut.poll(cx)).map(ResponseBody::wait_task).map_err(Into::into),
                    )));
                }
            }
        }
    }
}
