//! Provides response types
//!

use std::{convert::Infallible, io};

use serde::{Deserialize, Deserializer, Serialize, Serializer};

/// Response contains the output for the specific task, and error message if exists.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Response<TskOut, TskId> {
    /// The ID of the request being responded to.
    pub request_id: u64,
    /// The result of the response.
    pub result: Result<ResponseBody<TskOut, TskId>, ConsumerError>,
}

/// The request body type
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "method")]
pub enum ResponseBody<TskOut, TskId> {
    /// StartTask means start the specified task.
    StartTask { task_id: TskId },
    /// WaitTask means start the specified task.
    WaitTask { output: Option<TskOut> },
}

impl<TskOut, TskId> ResponseBody<TskOut, TskId> {
    pub fn start_task(task_id: TskId) -> Self {
        Self::StartTask { task_id }
    }

    pub fn wait_task(output: Option<TskOut>) -> Self {
        Self::WaitTask { output }
    }
}

impl<TskOut, TskId> Response<TskOut, TskId> {
    /// Creates new `Response` with given `request_id` and response `body`
    pub fn new(request_id: u64, result: Result<ResponseBody<TskOut, TskId>, ConsumerError>) -> Self {
        Self { request_id, result }
    }
}

/// An error indicating the server aborted the request early, e.g., due to request throttling.
#[derive(thiserror::Error, Clone, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[error("{kind:?}: {detail}")]
#[non_exhaustive]
pub struct ConsumerError {
    /// The type of error that occurred to fail the request.
    #[serde(serialize_with = "serialize_io_error_kind_as_u32")]
    #[serde(deserialize_with = "deserialize_io_error_kind_from_u32")]
    pub kind: io::ErrorKind,
    /// A message describing more detail about the error that occurred.
    pub detail: String,
}

impl From<io::Error> for ConsumerError {
    fn from(e: io::Error) -> Self {
        Self {
            kind: e.kind(),
            detail: e.to_string(),
        }
    }
}

impl From<Infallible> for ConsumerError {
    fn from(x: Infallible) -> Self {
        match x {}
    }
}

impl From<anyhow::Error> for ConsumerError {
    fn from(e: anyhow::Error) -> Self {
        ConsumerError {
            kind: io::ErrorKind::Other,
            detail: format!("{:?}", e),
        }
    }
}

/// Serializes [`io::ErrorKind`] as a `u32`.
#[allow(clippy::trivially_copy_pass_by_ref)] // Exact fn signature required by serde derive
pub fn serialize_io_error_kind_as_u32<S>(kind: &io::ErrorKind, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    use std::io::ErrorKind::*;
    match *kind {
        NotFound => 0,
        PermissionDenied => 1,
        ConnectionRefused => 2,
        ConnectionReset => 3,
        ConnectionAborted => 4,
        NotConnected => 5,
        AddrInUse => 6,
        AddrNotAvailable => 7,
        BrokenPipe => 8,
        AlreadyExists => 9,
        WouldBlock => 10,
        InvalidInput => 11,
        InvalidData => 12,
        TimedOut => 13,
        WriteZero => 14,
        Interrupted => 15,
        Other => 16,
        UnexpectedEof => 17,
        _ => 16,
    }
    .serialize(serializer)
}

/// Deserializes [`io::ErrorKind`] from a `u32`.
pub fn deserialize_io_error_kind_from_u32<'de, D>(deserializer: D) -> Result<io::ErrorKind, D::Error>
where
    D: Deserializer<'de>,
{
    use std::io::ErrorKind::*;
    Ok(match u32::deserialize(deserializer)? {
        0 => NotFound,
        1 => PermissionDenied,
        2 => ConnectionRefused,
        3 => ConnectionReset,
        4 => ConnectionAborted,
        5 => NotConnected,
        6 => AddrInUse,
        7 => AddrNotAvailable,
        8 => BrokenPipe,
        9 => AlreadyExists,
        10 => WouldBlock,
        11 => InvalidInput,
        12 => InvalidData,
        13 => TimedOut,
        14 => WriteZero,
        15 => Interrupted,
        16 => Other,
        17 => UnexpectedEof,
        _ => Other,
    })
}
