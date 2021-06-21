use std::path::Path;

use anyhow::{Error, Result};
use rocksdb::DB;

use super::{MetaDB, MetaError};

pub struct RocksMeta {
    inner: DB,
}

impl RocksMeta {
    pub fn open<P: AsRef<Path>>(p: P) -> Result<Self> {
        let inner = DB::open_default(p)?;
        Ok(RocksMeta { inner })
    }
}

impl MetaDB for RocksMeta {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()> {
        self.inner.put(key.as_ref().as_bytes(), value)?;
        Ok(())
    }

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool> {
        self.inner
            .get_pinned(key.as_ref().as_bytes())
            .map(|r| r.is_some())
            .map_err(From::from)
    }

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R, MetaError>
    where
        F: FnOnce(&[u8]) -> Result<R>,
    {
        let bytes = self
            .inner
            .get_pinned(key.as_ref().as_bytes())
            .map_err(|e| MetaError::from(Error::new(e)))?;
        match bytes {
            Some(b) => cb(b.as_ref()).map_err(From::from),
            None => Err(MetaError::NotFound),
        }
    }

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()> {
        self.inner.delete(key.as_ref().as_bytes())?;
        Ok(())
    }
}
