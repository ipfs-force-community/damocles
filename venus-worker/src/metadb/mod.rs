use anyhow::Result;

pub trait MetaDB {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()>;

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool>;

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R>
    where
        F: FnOnce(&[u8]) -> Result<R>;

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()>;

    fn get<K: AsRef<str>>(&self, key: K) -> Result<Vec<u8>> {
        self.view(key, |b| Ok(b.to_owned()))
    }
}

pub struct PrefixedMetaDB<'p, DB: MetaDB> {
    prefix: String,
    inner: &'p DB,
}

impl<'p, DB: MetaDB> PrefixedMetaDB<'p, DB> {
    pub fn wrap<P: Into<String>>(prefix: P, inner: &'p DB) -> Self {
        Self {
            prefix: prefix.into(),
            inner,
        }
    }

    fn key<K: AsRef<str>>(&self, k: K) -> String {
        [&self.prefix, k.as_ref()].join("/")
    }
}

impl<'p, DB: MetaDB> MetaDB for PrefixedMetaDB<'p, DB> {
    fn set<K: AsRef<str>, V: AsRef<[u8]>>(&self, key: K, value: V) -> Result<()> {
        self.inner.set(self.key(key), value)
    }

    fn has<K: AsRef<str>>(&self, key: K) -> Result<bool> {
        self.inner.has(self.key(key))
    }

    fn view<K: AsRef<str>, F, R>(&self, key: K, cb: F) -> Result<R>
    where
        F: FnOnce(&[u8]) -> Result<R>,
    {
        self.inner.view(self.key(key), cb)
    }

    fn remove<K: AsRef<str>>(&self, key: K) -> Result<()> {
        self.inner.remove(self.key(key))
    }
}
