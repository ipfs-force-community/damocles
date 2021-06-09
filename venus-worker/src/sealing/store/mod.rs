use std::path::PathBuf;

use crate::metadb::MetaDB;

struct Store<DB: MetaDB> {
    pub path: PathBuf,
    pub reserved_capacity: usize,

    pub meta: DB,
}
