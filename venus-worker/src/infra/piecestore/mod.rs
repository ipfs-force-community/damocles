use std::io::Read;

pub mod proxy;

use anyhow::{anyhow, Result};
use fil_types::UnpaddedPieceSize;
use forest_cid::Cid;
use reqwest::Url;

pub trait PieceStore: Send + Sync {
    fn get(&self, c: &Cid, payload_size: u64, target_size: UnpaddedPieceSize) -> Result<Box<dyn Read>>;

    fn url(&self, c: &Cid) -> Url;
}

/// EmptyPieceStore always returns None
pub struct EmptyPieceStore;

impl PieceStore for EmptyPieceStore {
    fn get(&self, _c: &Cid, _payload_size: u64, _target_size: UnpaddedPieceSize) -> Result<Box<dyn Read>> {
        Err(anyhow!("empty piece store"))
    }

    fn url(&self, _c: &Cid) -> Url {
        Url::parse("http://unreachable").unwrap()
    }
}
