//! abstractions & implementations for object store

use std::io::{self, Read};
use std::path::Path;

pub mod filestore;

/// errors in object storage usage
pub enum ObjectStoreError {
    /// io errors
    IO(io::Error),

    /// other errors
    Other(anyhow::Error),
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
pub trait ObjectStore {
    /// get should return a reader for the given path
    fn get<P: AsRef<Path>>(&self, path: P) -> ObjResult<Box<dyn Read>>;

    /// put an object
    fn put<P: AsRef<Path>, R: Read>(&self, path: P, r: R) -> ObjResult<u64>;

    /// get specified pieces
    fn get_chunks<P: AsRef<Path>>(
        &self,
        path: P,
        ranges: &[Range],
    ) -> ObjResult<Box<dyn Iterator<Item = ObjResult<Box<dyn Read>>>>>;
}
