pub mod local;
pub mod remote;

use forest_cid::Cid;
use vc_processors::builtin::tasks::PieceFile;

pub trait PieceStore: Send + Sync {
    fn get(&self, c: &Cid) -> Option<PieceFile>;
}

pub struct ComposePieceStore<P1, P2> {
    first: P1,
    second: P2,
}

impl<P1, P2> ComposePieceStore<P1, P2> {
    pub fn new(first: P1, second: P2) -> Self {
        Self { first, second }
    }
}

impl<P1, P2> PieceStore for ComposePieceStore<P1, P2>
where
    P1: PieceStore,
    P2: PieceStore,
{
    fn get(&self, c: &Cid) -> Option<PieceFile> {
        if let Some(f) = self.first.get(c) {
            return Some(f);
        }
        self.second.get(c)
    }
}
