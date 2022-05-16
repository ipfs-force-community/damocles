use std::fs::OpenOptions;
use std::path::PathBuf;

use anyhow::{Context, Result};
use filecoin_proofs::{get_base_tree_leafs, get_base_tree_size, DefaultBinaryTree, DefaultPieceHasher, StoreConfig, BINARY_ARITY};
use filecoin_proofs_api::RegisteredSealProof;
use memmap::{Mmap, MmapOptions};
use storage_proofs_core::{
    cache_key::CacheKey,
    merkle::{create_base_merkle_tree, BinaryMerkleTree},
    util::default_rows_to_discard,
};

enum Bytes {
    Mmap(Mmap),
    InMem(Vec<u8>),
}

impl AsRef<[u8]> for Bytes {
    #[inline]
    fn as_ref(&self) -> &[u8] {
        match self {
            Bytes::Mmap(m) => &m[..],
            Bytes::InMem(m) => &m[..],
        }
    }
}

pub fn create_tree_d(registered_proof: RegisteredSealProof, in_path: Option<PathBuf>, cache_path: PathBuf) -> Result<()> {
    let sector_size = registered_proof.sector_size();
    let tree_size = get_base_tree_size::<DefaultBinaryTree>(sector_size)?;
    let tree_leafs = get_base_tree_leafs::<DefaultBinaryTree>(tree_size)?;

    let data = match in_path {
        Some(p) => {
            let f = OpenOptions::new()
                .read(true)
                .open(&p)
                .with_context(|| format!("open staged file {:?}", p))?;

            let mapped = unsafe { MmapOptions::new().map(&f).with_context(|| format!("mmap staged file: {:?}", p))? };

            Bytes::Mmap(mapped)
        }

        None => Bytes::InMem(vec![0; sector_size.0 as usize]),
    };

    let cfg = StoreConfig::new(
        &cache_path,
        CacheKey::CommDTree.to_string(),
        default_rows_to_discard(tree_leafs, BINARY_ARITY),
    );

    create_base_merkle_tree::<BinaryMerkleTree<DefaultPieceHasher>>(Some(cfg), tree_leafs, data.as_ref())?;

    Ok(())
}
