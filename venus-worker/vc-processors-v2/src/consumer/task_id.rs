use std::hash::Hasher;

use serde::{de::DeserializeOwned, Serialize};

pub trait Generator<Tsk>: Clone {
    type TaskId: Serialize + DeserializeOwned + Clone;

    fn generate(&self, task: &Tsk) -> Self::TaskId;
}

#[derive(Debug, Clone)]
pub struct XXHash;

impl<Tsk: serde::Serialize> Generator<Tsk> for XXHash {
    type TaskId = u64;

    fn generate(&self, task: &Tsk) -> Self::TaskId {
        use twox_hash::XxHash64;

        // TODO: error handing
        let data = bincode::serialize(task).unwrap();

        let mut hasher = XxHash64::with_seed(0);
        hasher.write(&data);
        hasher.finish()
    }
}
