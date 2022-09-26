pub mod local;
pub mod remote;

use forest_cid::Cid;
use vc_processors::builtin::tasks::PieceFile;

pub trait PieceStore: Send + Sync {
    fn get(&self, c: &Cid) -> PieceFile;
}
