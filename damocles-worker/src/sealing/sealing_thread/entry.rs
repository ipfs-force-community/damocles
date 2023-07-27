use std::fmt;
use std::fs::{create_dir_all, File, OpenOptions, ReadDir as fsReadDir};
use std::iter::Iterator;
use std::path::{Path, PathBuf};

use anyhow::{anyhow, Context, Result};

pub struct ReadDir {
    base: PathBuf,
    inner: fsReadDir,
}

impl Iterator for ReadDir {
    type Item = Result<Entry>;

    fn next(&mut self) -> Option<Self::Item> {
        self.inner.next().map(|item| {
            let item = item.context("invalid dir entry")?;
            Entry::from_full(item.path(), &self.base)
        })
    }
}

pub enum Entry {
    Dir(PathBuf, (PathBuf, PathBuf)),
    File(PathBuf, (PathBuf, PathBuf)),
}

impl Entry {
    #[inline]
    fn file_path(&self) -> Result<&PathBuf> {
        match self {
            Entry::Dir(p, (_, _)) => Err(anyhow!("{:?} is a dir", p)),
            Entry::File(p, (_, _)) => Ok(p),
        }
    }

    #[inline]
    fn dir_path(&self) -> Result<&PathBuf> {
        match self {
            Entry::Dir(p, (_, _)) => Ok(p),
            Entry::File(p, (_, _)) => Err(anyhow!("{:?} is a file", p)),
        }
    }

    #[inline]
    fn from_full(full: PathBuf, base: &PathBuf) -> Result<Self> {
        let rel = full
            .strip_prefix(base)
            .with_context(|| format!("prefix not match {:?} <> {:?}", full, base))?
            .to_path_buf();

        Ok(if full.is_file() {
            Entry::File(full, (base.clone(), rel))
        } else {
            Entry::Dir(full, (base.clone(), rel))
        })
    }

    pub fn dir(base: &Path, rel: PathBuf) -> Self {
        let full = base.join(&rel);
        Entry::Dir(full, (base.to_path_buf(), rel))
    }

    pub fn file(base: &Path, rel: PathBuf) -> Self {
        let full = base.join(&rel);
        Entry::File(full, (base.to_path_buf(), rel))
    }

    pub fn base(&self) -> &PathBuf {
        match self {
            Entry::Dir(_, (p, _)) => p,
            Entry::File(_, (p, _)) => p,
        }
    }

    pub fn rel(&self) -> &PathBuf {
        match self {
            Entry::Dir(_, (_, p)) => p,
            Entry::File(_, (_, p)) => p,
        }
    }

    pub fn full(&self) -> &PathBuf {
        match self {
            Entry::Dir(p, (_, _)) => p,
            Entry::File(p, (_, _)) => p,
        }
    }

    pub fn join<P: AsRef<Path>>(&self, part: P) -> Entry {
        match self {
            Entry::Dir(p, (base, rel)) => Entry::Dir(p.join(part.as_ref()), (base.clone(), rel.join(part.as_ref()))),
            Entry::File(p, (base, rel)) => Entry::File(p.join(part.as_ref()), (base.clone(), rel.join(part.as_ref()))),
        }
    }

    pub fn read_dir(&self) -> Result<ReadDir> {
        let p = self.dir_path()?;
        let inner = p.read_dir().with_context(|| format!("read dir for {:?}", p))?;

        Ok(ReadDir {
            base: self.base().clone(),
            inner,
        })
    }

    pub fn prepare(&self) -> Result<()> {
        match self {
            Entry::Dir(p, (_, _)) => create_dir_all(p).with_context(|| format!("create dir for {:?}", p)),

            Entry::File(p, (_, _)) => {
                p.parent()
                    .map(|d| create_dir_all(d).with_context(|| format!("create parent dir for {:?}", p)))
                    .transpose()?;

                Ok(())
            }
        }
    }

    pub fn init_file(&self) -> Result<File> {
        let p = self.file_path()?;
        self.prepare()?;
        OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(true)
            .open(p)
            .with_context(|| format!("create file for {:?}", p))
    }
}

impl fmt::Debug for Entry {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Entry::Dir(_, (b, r)) => {
                write!(f, "dir ({:?}, {:?})", b, r)
            }

            Entry::File(_, (b, r)) => {
                write!(f, "file ({:?}, {:?})", b, r)
            }
        }
    }
}

impl From<Entry> for PathBuf {
    fn from(ent: Entry) -> Self {
        match ent {
            Entry::Dir(p, (_, _)) => p,
            Entry::File(p, (_, _)) => p,
        }
    }
}

impl AsRef<PathBuf> for Entry {
    fn as_ref(&self) -> &PathBuf {
        self.full()
    }
}

impl AsRef<Path> for Entry {
    fn as_ref(&self) -> &Path {
        self.full()
    }
}
