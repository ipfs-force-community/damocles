use anyhow::{Error, Result};
use serde::{Deserialize, Serialize};
use serde_json::{from_slice, to_vec};
use tracing::error;

pub mod rocks;

pub enum MetaError {
    NotFound,
    Failure(Error),
}

impl From<Error> for MetaError {
    fn from(val: Error) -> Self {
        MetaError::Failure(val)
    }
}

pub struct MetaDocumentDB<M>(M);

impl<M: MetaDB> MetaDocumentDB<M> {
    pub fn wrap(inner: M) -> Self {
        MetaDocumentDB(inner)
    }

    pub fn set<K, T>(&self, key: K, val: &T) -> Result<()>
    where
        K: AsRef<str>,
        T: Serialize,
    {
        let data = to_vec(val)?;
        self.0.set(key, data)
    }

    pub fn get<K, T>(&self, key: K) -> Result<T, MetaError>
    where
        K: AsRef<str>,
        T: for<'a> Deserialize<'a>,
    {
        self.0.view(key, |b: &[u8]| from_slice(b).map_err(Error::new))
    }

    pub fn remove<K: AsRef<str>>(&self, key: K) -> Result<()> {
        self.0.remove(key)
    }
}

pub trait MetaDB {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()>;

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool>;

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R, MetaError>
    where
        F: FnOnce(&[u8]) -> Result<R>;

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()>;

    fn get<K: AsRef<str>>(&self, key: K) -> Result<Vec<u8>, MetaError> {
        self.view(key, |b| Ok(b.to_owned()))
    }
}

impl<T: MetaDB> MetaDB for &T {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()> {
        (*self).set(key, value)
    }

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool> {
        (*self).has(key)
    }

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R, MetaError>
    where
        F: FnOnce(&[u8]) -> Result<R>,
    {
        (*self).view(key, cb)
    }

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()> {
        (*self).remove(key)
    }
}

pub struct PrefixedMetaDB<DB: MetaDB> {
    prefix: String,
    inner: DB,
}

impl<DB: MetaDB> PrefixedMetaDB<DB> {
    pub fn wrap<P: Into<String>>(prefix: P, inner: DB) -> Self {
        Self {
            prefix: prefix.into(),
            inner,
        }
    }

    fn key<K: AsRef<str>>(&self, k: K) -> String {
        [&self.prefix, k.as_ref()].join("/")
    }
}

impl<DB: MetaDB> MetaDB for PrefixedMetaDB<DB> {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()> {
        self.inner.set(self.key(key), value)
    }

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool> {
        self.inner.has(self.key(key))
    }

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R, MetaError>
    where
        F: FnOnce(&[u8]) -> Result<R>,
    {
        self.inner.view(self.key(key), cb)
    }

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()> {
        self.inner.remove(self.key(key))
    }
}

/// MaybeDirty is a wrapper type that marks whether the internal object has been borrowed mutably.
/// Additional sync operations can be avoided if the internal data is not mutably borrowed
pub struct MaybeDirty<T> {
    inner: T,
    is_dirty: bool,
}

impl<T> MaybeDirty<T> {
    pub fn new(inner: T) -> Self {
        Self { inner, is_dirty: false }
    }

    pub fn is_dirty(&self) -> bool {
        self.is_dirty
    }

    pub fn sync(&mut self) {
        self.is_dirty = false;
    }
}

impl<T> core::ops::Deref for MaybeDirty<T> {
    type Target = T;

    fn deref(&self) -> &Self::Target {
        &self.inner
    }
}

impl<T> core::ops::DerefMut for MaybeDirty<T> {
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.is_dirty = true;
        &mut self.inner
    }
}

impl<T> Drop for MaybeDirty<T> {
    fn drop(&mut self) {
        assert!(!self.is_dirty, "data dirty when dropping");
    }
}

pub struct Saved<T, K, DB>
where
    T: Default + Serialize,
    K: AsRef<str>,
    DB: MetaDB,
{
    // Using MaybeDirty can avoid some unnecessary `db.set` operations
    data: MaybeDirty<T>,
    key: K,
    db: MetaDocumentDB<DB>,
}

impl<T, K, DB> Saved<T, K, DB>
where
    T: Default + Serialize,
    K: AsRef<str>,
    DB: MetaDB,
{
    pub fn load(key: K, db: DB) -> anyhow::Result<Self>
    where
        T: for<'a> Deserialize<'a>,
    {
        let db = MetaDocumentDB::wrap(db);

        let data = db.get(&key).or_else(|e| match e {
            MetaError::NotFound => Ok(T::default()),
            MetaError::Failure(ie) => Err(ie),
        })?;

        Ok(Self {
            data: MaybeDirty::new(data),
            key,
            db,
        })
    }

    pub fn sync(&mut self) -> anyhow::Result<()> {
        if !self.data.is_dirty() {
            return Ok(());
        }
        self.db.set(&self.key, &*self.data)?;
        self.data.sync();
        Ok(())
    }

    pub fn delete(&mut self) -> anyhow::Result<()> {
        self.db.remove(&self.key)?;
        *self.data = Default::default();
        self.data.sync();
        Ok(())
    }

    pub fn inner_mut(&mut self) -> &mut MaybeDirty<T> {
        &mut self.data
    }
}

impl<T, K, DB> core::ops::Deref for Saved<T, K, DB>
where
    T: Default + Serialize,
    K: AsRef<str>,
    DB: MetaDB,
{
    type Target = T;

    fn deref(&self) -> &Self::Target {
        self.data.deref()
    }
}

impl<T, K, DB> core::ops::DerefMut for Saved<T, K, DB>
where
    T: Default + Serialize,
    K: AsRef<str>,
    DB: MetaDB,
{
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.data.deref_mut()
    }
}

impl<T, K, DB> Drop for Saved<T, K, DB>
where
    T: Default + Serialize,
    K: AsRef<str>,
    DB: MetaDB,
{
    fn drop(&mut self) {
        if let Err(e) = self.sync() {
            error!(err = ?e, "sync data");
        }
    }
}

#[cfg(test)]
mod tests {
    use pretty_assertions::assert_eq;

    use super::MaybeDirty;

    #[test]
    fn test_maybe_dirty() {
        let mut x = String::from("glgjssy");
        let mut md = MaybeDirty::new(x.clone());
        assert_eq!(x, *md);
        assert!(!md.is_dirty());

        md.push_str(", qyhfbqz. ");
        assert_ne!(x, *md);
        assert!(md.is_dirty());
        x.push_str(", qyhfbqz. ");

        md.sync();
        assert!(!md.is_dirty());

        md.push_str("sometimes");
        assert!(md.is_dirty());
        x.push_str("sometimes");

        assert_eq!(x, *md);

        // avoid panic
        md.sync();
    }
}
