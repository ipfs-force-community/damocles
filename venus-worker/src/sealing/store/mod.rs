use std::path::PathBuf;

use crate::metadb::MetaDB;

pub struct Store<DB: MetaDB> {
    pub path: PathBuf,
    pub reserved_capacity: usize,

    pub meta: DB,
}
