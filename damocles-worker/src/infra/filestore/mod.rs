//! abstractions & implementations for object store

use std::collections::HashMap;
use std::error::Error;
use std::fmt;
use std::fs::create_dir_all;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use vc_processors::fil_proofs::{ActorID, SectorId};

use crate::rpc::sealer::SectorID;
use crate::rpc::sealer::{PathType, SealerClient};
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

/// definition of object store
pub trait FileStore: Send + Sync {
    /// instance name of the store
    fn instance(&self) -> String;

    /// get paths of given sectors.
    fn sector_paths(
        &self,
        path_type: PathType,
        miner_id: ActorID,
        sector_numbers: Vec<SectorId>,
    ) -> StoreResult<Vec<PathBuf>>;

    /// get path of given custom name.
    fn custom_path(&self, custom: String) -> StoreResult<PathBuf>;

    /// if this instance is read-only
    fn readonly(&self) -> bool;
}

/// Extension methods of Objstore
pub trait FileStoreExt {
    /// Get a single sector path
    fn sector_path(
        &self,
        path_type: PathType,
        sid: SectorID,
    ) -> StoreResult<PathBuf>;
}

impl<T: FileStore + ?Sized> FileStoreExt for T {
    fn sector_path(
        &self,
        path_type: PathType,
        sid: SectorID,
    ) -> StoreResult<PathBuf> {
        let mut res =
            self.sector_paths(path_type, sid.miner, vec![sid.number.into()])?;
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

    fn sector_paths(
        &self,
        path_type: PathType,
        miner_id: ActorID,
        sector_numbers: Vec<SectorId>,
    ) -> StoreResult<Vec<PathBuf>> {
        call_rpc! {
            self.rpc=>store_sector_sub_paths(self.instance(), path_type, miner_id as u64, sector_numbers,)
        }
        .map(|p| p.iter().map(|p| self.local_path.join(p)).collect())
        .map_err(|e| FileStoreError::Other(e.1))
    }

    fn custom_path(&self, custom: String) -> StoreResult<PathBuf> {
        call_rpc! {
            self.rpc=>store_custom_sub_path(self.instance(), custom,)
        }
        .map(|p| self.local_path.join(p))
        .map_err(|e| FileStoreError::Other(e.1))
    }

    fn readonly(&self) -> bool {
        self.readonly
    }
}
