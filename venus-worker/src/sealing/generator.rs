use anyhow::Result;

use filecoin_proofs::{
    get_base_tree_leafs, get_base_tree_size, DefaultBinaryTree, DefaultPieceHasher, SectorSize,
    StoreConfig, BINARY_ARITY,
};

use storage_proofs_core::{
    cache_key::CacheKey,
    merkle::{create_base_merkle_tree, BinaryMerkleTree},
    util::default_rows_to_discard,
};

/// generate tree-d file for cc sector
pub fn generate_static_tree_d(sector_size: u64, path: String) -> Result<()> {
    let sz = SectorSize::from(sector_size as u64);

    let tree_size = get_base_tree_size::<DefaultBinaryTree>(SectorSize::from(sz))?;
    let tree_leafs = get_base_tree_leafs::<DefaultBinaryTree>(tree_size)?;

    let data = vec![0; sz.0 as usize];

    let cfg = StoreConfig::new(
        &path,
        CacheKey::CommDTree.to_string(),
        default_rows_to_discard(tree_leafs, BINARY_ARITY),
    );

    create_base_merkle_tree::<BinaryMerkleTree<DefaultPieceHasher>>(Some(cfg), tree_leafs, &data)?;

    Ok(())
}
