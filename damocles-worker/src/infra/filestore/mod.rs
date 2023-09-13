//! abstractions & implementations for object store

use std::collections::HashMap;
use std::error::Error;
use std::fmt;
use std::fs::create_dir_all;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{anyhow, Context, Result};

use crate::rpc::sealer::SealerClient;
use crate::rpc::sealer::SectorID;
use crate::rpc::sealer::StoreResource;
use crate::sealing::call_rpc;

pub mod attached;

/// errors in object storage usage
#[derive(Debug)]
pub enum FileStoreError {
    /// io errors
    IO(io::Error),

    /// other errors
    Other(anyhow::Error),
}

impl fmt::Display for FileStoreError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Self::IO(inner) => write!(f, "file store io err: {}", inner),
            Self::Other(inner) => write!(f, "file store err: {}", inner),
        }
    }
}

impl Error for FileStoreError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::IO(inner) => Some(inner),
            Self::Other(inner) => Some(inner.root_cause()),
        }
    }
}

impl From<io::Error> for FileStoreError {
    fn from(val: io::Error) -> FileStoreError {
        FileStoreError::IO(val)
    }
}

impl From<anyhow::Error> for FileStoreError {
    fn from(val: anyhow::Error) -> FileStoreError {
        FileStoreError::Other(val)
    }
}

/// type alias for Result<T, ObjectStoreError>
pub type StoreResult<T> = Result<T, FileStoreError>;

/// for pieces
#[derive(Debug, Copy, Clone)]
pub struct Range {
    /// offset in the object, from start
    pub offset: u64,

    /// piece size
    pub size: u64,
}

/// Filestore Resource
#[allow(missing_docs)]
#[derive(Debug, Clone)]
pub enum Resource {
    Sealed(SectorID),
    Update(SectorID),
    Cache(SectorID),
    UpdateCache(SectorID),
    Custom(String),
}

impl Resource {
    /// Returns None if resource is Custom otherwise Some(sector_id)
    pub fn sector_id(&self) -> Option<&SectorID> {
        match self {
            Resource::Sealed(sid)
            | Resource::Update(sid)
            | Resource::Cache(sid)
            | Resource::UpdateCache(sid) => Some(sid),
            Resource::Custom(_) => None,
        }
    }
}

/// definition of object store
pub trait FileStore: Send + Sync {
    /// instance name of the store
    fn instance(&self) -> String;

    /// get paths of the given resources.
    fn paths(&self, resources: Vec<Resource>) -> StoreResult<Vec<PathBuf>>;

    /// if this instance is read-only
    fn readonly(&self) -> bool;
}

/// Extension methods of Objstore
pub trait FileStoreExt {
    /// Get a single uri
    fn path(&self, resource: Resource) -> StoreResult<PathBuf>;
}

impl<T: FileStore + ?Sized> FileStoreExt for T {
    fn path(&self, resource: Resource) -> StoreResult<PathBuf> {
        let mut res = self.paths(vec![resource])?;
        Ok(res.swap_remove(0))
    }
}

/// DefaultFileStore
pub struct DefaultFileStore {
    local_path: PathBuf,
    instance: String,
    readonly: bool,
    rpc: Arc<SealerClient>,
}

impl DefaultFileStore {
    /// init filestore, create a placeholder file in its root dir
    pub fn init<P: AsRef<Path>>(p: P) -> Result<()> {
        create_dir_all(p.as_ref())?;

        Ok(())
    }

    /// open the file store at given path
    pub fn open<P: AsRef<Path>>(
        p: P,
        ins: Option<String>,
        readonly: bool,
        rpc: Arc<SealerClient>,
    ) -> Result<Self> {
        let dir_path =
            p.as_ref().canonicalize().context("canonicalize dir path")?;
        if !dir_path
            .metadata()
            .context("read dir metadata")
            .map(|meta| meta.is_dir())?
        {
            return Err(anyhow!("base path of the file store should a dir"));
        };

        let instance =
            match ins.or_else(|| dir_path.to_str().map(|s| s.to_owned())) {
                Some(i) => i,
                None => {
                    return Err(anyhow!(
                        "dir path {:?} may contain invalid utf8 chars",
                        dir_path
                    ))
                }
            };

        Ok(Self {
            local_path: dir_path,
            instance,
            readonly,
            rpc,
        })
    }
}

impl FileStore for DefaultFileStore {
    fn instance(&self) -> String {
        self.instance.clone()
    }

    fn paths(&self, resources: Vec<Resource>) -> StoreResult<Vec<PathBuf>> {
        let store_resources = resources
            .into_iter()
            .map(|r| match r {
                Resource::Sealed(sid) => StoreResource {
                    path_type: crate::rpc::sealer::PathType::Sealed,
                    sector_id: Some(sid),
                    custom: None,
                },
                Resource::Update(sid) => StoreResource {
                    path_type: crate::rpc::sealer::PathType::Sealed,
                    sector_id: Some(sid),
                    custom: None,
                },
                Resource::Cache(sid) => StoreResource {
                    path_type: crate::rpc::sealer::PathType::Sealed,
                    sector_id: Some(sid),
                    custom: None,
                },
                Resource::UpdateCache(sid) => StoreResource {
                    path_type: crate::rpc::sealer::PathType::UpdateCache,
                    sector_id: Some(sid),
                    custom: None,
                },
                Resource::Custom(s) => StoreResource {
                    path_type: crate::rpc::sealer::PathType::Custom,
                    sector_id: None,
                    custom: Some(s),
                },
            })
            .collect();
        call_rpc! {
            self.rpc=>store_sub_paths(self.instance(), store_resources,)
        }
        .map(|p| p.iter().map(|p| self.local_path.join(p)).collect())
        .map_err(|e| FileStoreError::Other(e.1))
    }

    fn readonly(&self) -> bool {
        self.readonly
    }
}
