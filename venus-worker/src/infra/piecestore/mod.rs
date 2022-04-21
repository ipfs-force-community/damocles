use std::io::Read;

pub mod proxy;

use anyhow::Result;
use fil_types::UnpaddedPieceSize;
use forest_cid::Cid;

pub trait PieceStore: Send + Sync {
    fn get(&self, c: Cid, payload_size: u64, target_size: UnpaddedPieceSize) -> Result<Box<dyn Read>>;
}
