use std::io;

use tokio_transports::{
    framed::{FramedExt, LinesCodec},
    rw::ReadWriterExt,
    serded::{Json, Serded},
};
use tokio_util::codec::Framed;

pub use tokio_transports::rw::pipe;

use crate::{Request, Response, Task};

type Req<Tsk, TskId> = Request<Tsk, TskId>;
type Resp<Tsk, TskId> = Response<<Tsk as Task>::Output, TskId>;
type ListenJsonSerded<Tsk, TskId> = Json<Req<Tsk, TskId>, Resp<Tsk, TskId>>;
type ConnectJsonSerded<Tsk, TskId> = Json<Resp<Tsk, TskId>, Req<Tsk, TskId>>;

pub fn listen<Tsk: Task, TskId>(
) -> Serded<Framed<pipe::ListenStdio, LinesCodec>, ListenJsonSerded<Tsk, TskId>, Req<Tsk, TskId>, Resp<Tsk, TskId>> {
    pipe::listen().framed_default().serded_default()
}

pub async fn listen_ready_message<Tsk: Task, TskId>(
    ready_message: impl AsRef<str>,
) -> io::Result<Serded<Framed<pipe::ListenStdio, LinesCodec>, ListenJsonSerded<Tsk, TskId>, Req<Tsk, TskId>, Resp<Tsk, TskId>>> {
    Ok(pipe::listen_ready_message(ready_message).await?.framed_default().serded_default())
}

pub fn connect<Tsk: Task, TskId>(
    cmd: pipe::Command,
) -> Serded<Framed<pipe::ChildProcess, LinesCodec>, ConnectJsonSerded<Tsk, TskId>, Resp<Tsk, TskId>, Req<Tsk, TskId>> {
    pipe::connect(cmd).framed_default().serded_default()
}
