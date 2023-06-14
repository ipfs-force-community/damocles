//! abstractions & implementations for object store

use std::error::Error;
use std::fmt;
use std::io;
use std::path::{Path, PathBuf};

pub mod attached;
pub mod filestore;

/// errors in object storage usage
#[derive(Debug)]
pub enum ObjectStoreError {
    /// io errors
    IO(io::Error),

    /// other errors
    Other(anyhow::Error),
}

impl fmt::Display for ObjectStoreError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Self::IO(inner) => write!(f, "obj store io err: {}", inner),
            Self::Other(inner) => write!(f, "obj store err: {}", inner),
        }
    }
}

impl Error for ObjectStoreError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::IO(inner) => Some(inner),
            Self::Other(inner) => Some(inner.root_cause()),
        }
    }
}

impl From<io::Error> for ObjectStoreError {
    fn from(val: io::Error) -> ObjectStoreError {
        ObjectStoreError::IO(val)
    }
}

impl From<anyhow::Error> for ObjectStoreError {
    fn from(val: anyhow::Error) -> ObjectStoreError {
        ObjectStoreError::Other(val)
    }
}

/// type alias for Result<T, ObjectStoreError>
pub type ObjResult<T> = Result<T, ObjectStoreError>;

/// for pieces
#[derive(Debug, Copy, Clone)]
pub struct Range {
    /// offset in the object, from start
    pub offset: u64,

    /// piece size
    pub size: u64,
}

/// definition of object store
pub trait ObjectStore: Send + Sync {
    /// instance name of the store
    fn instance(&self) -> String;

    /// unique identifier of the given resource.
    /// for fs-like stores, this should return an abs path.
    /// for other stores, this may return a url, or path part of a url.
    fn uri(&self, resource: &Path) -> ObjResult<PathBuf>;

    /// if this instance is read-only
    fn readonly(&self) -> bool;
}
