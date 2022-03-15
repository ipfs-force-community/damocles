//! abstractions & implementations for object store

use std::error::Error;
use std::fmt;
use std::io::{self, Read};
use std::path::Path;

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

    /// get should return a reader for the given path
    fn get(&self, path: &Path) -> ObjResult<Box<dyn Read>>;

    /// put an object
    fn put(&self, path: &Path, r: Box<dyn Read>) -> ObjResult<u64>;

    /// get specified pieces
    fn get_chunks(
        &self,
        path: &Path,
        ranges: &[Range],
    ) -> ObjResult<Box<dyn Iterator<Item = ObjResult<Box<dyn Read>>>>>;

    /// copy an object to a local path
    fn copy_to(&self, path: &Path, dest: &Path, allow_sym: bool) -> ObjResult<()>;

    /// if this instance is read-only
    fn readonly(&self) -> bool;
}
