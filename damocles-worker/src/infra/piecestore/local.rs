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
            let filename = c.to_string();
            let path = dir.join(&filename);
            tracing::debug!("load local piece: {}", path.display());
            if path.exists() {
                return Some(PieceFile::Local(path));
            }
            let path = path.with_file_name(format!("{}.car", filename));
            if path.exists() {
                return Some(PieceFile::Local(path));
            }
        }
        None
    }
}

mod tests {
    use std::str::FromStr;

    use forest_cid::Cid;

    use crate::infra::piecestore::PieceStore;

    use super::LocalPieceStore;

    #[test]
    fn test_get_piece() {
        let tmpdir = tempfile::tempdir().unwrap();

        let a_cid = Cid::from_str(
            "baga6ea4seaqdb5jftgpyv2rsxatevpzfb5i5747roq57jb2n3mnkjmz3etcreda",
        )
        .unwrap();

        let b_cid = Cid::from_str(
            "baga6ea4seaqhw7mv6ezdt3plvh254sqbagyzabkemlbhwvqsl4o4xksceeb4uoa",
        )
        .unwrap();

        std::fs::write(tmpdir.path().join(a_cid.to_string()), "test_a")
            .unwrap();
        std::fs::write(tmpdir.path().join(format!("{}.car", b_cid)), "test_b")
            .unwrap();

        let store = LocalPieceStore::new(vec![tmpdir.into_path()]);
        assert!(store.get(&a_cid).is_some());
        assert!(store.get(&b_cid).is_some());
    }
}
