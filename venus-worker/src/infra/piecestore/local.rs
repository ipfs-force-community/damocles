use std::path::PathBuf;

use super::PieceStore;
use forest_cid::Cid;
use vc_processors::builtin::tasks::PieceFile;

pub struct LocalPieceStore {
    base: PathBuf,
}

impl LocalPieceStore {
    pub fn new(base: impl Into<PathBuf>) -> Self {
        Self { base: base.into() }
    }
}

impl PieceStore for LocalPieceStore {
    fn get(&self, c: &Cid) -> Option<PieceFile> {
        let path = self.base.join(c.to_string());
        tracing::debug!("load local piece: {}", path.display());
        if path.exists() {
            Some(PieceFile::Local(path))
        } else {
            None
        }
    }
}
