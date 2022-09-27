use anyhow::{anyhow, Context, Result};
use forest_cid::Cid;
use reqwest::Url;
use vc_processors::builtin::tasks::PieceFile;

use super::PieceStore;

const ENDPOINT: &str = "piecestore";

pub struct RemotePieceStore {
    base: Url,
}

impl RemotePieceStore {
    pub fn new(host: &str) -> Result<Self> {
        let mut base = Url::parse(host).context("parse host")?;
        base.path_segments_mut()
            .map_err(|_| anyhow!("url cannot be a base"))?
            .push(ENDPOINT);

        Ok(Self { base })
    }
}

impl PieceStore for RemotePieceStore {
    fn get(&self, c: &Cid) -> Option<PieceFile> {
        let mut u = self.base.clone();
        u.path_segments_mut().unwrap().push(&c.to_string());
        Some(PieceFile::Url(u.as_str().to_string()))
    }
}
