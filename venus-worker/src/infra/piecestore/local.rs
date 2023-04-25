use std::path::PathBuf;

use super::PieceStore;
use forest_cid::Cid;
use vc_processors::builtin::tasks::PieceFile;

pub struct LocalPieceStore {
    dirs: Vec<PathBuf>,
}

impl LocalPieceStore {
    pub fn new(dirs: Vec<PathBuf>) -> Self {
        Self { dirs }
    }
}

impl PieceStore for LocalPieceStore {
    fn get(&self, c: &Cid) -> Option<PieceFile> {
        for dir in &self.dirs {
            let path = dir.join(c.to_string());
            tracing::debug!("load local piece: {}", path.display());
            if path.exists() {
                return Some(PieceFile::Local(path));
            }
        }
        None
    }
}
