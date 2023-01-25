pub use md5_bincode::Md5BincodeTaskIdExtractor;
pub use md5_json::Md5JsonTaskIdExtractor;

use serde::Serialize;

use crate::core::{TaskId, TaskIdExtractor};

/// TODO: doc
pub mod md5_bincode {
    use std::io;

    use md5::{Digest, Md5};

    use super::*;

    /// TODO: doc
    #[derive(Default, Clone)]
    pub struct Md5BincodeTaskIdExtractor;

    impl<T: Serialize> TaskIdExtractor<T> for Md5BincodeTaskIdExtractor {
        type Error = io::Error;

        fn extract(&self, task: &T) -> Result<TaskId, Self::Error> {
            let bytes = bincode::serialize(task).map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
            let mut hasher = Md5::new();
            hasher.update(bytes);
            Ok(format!("{:x}", hasher.finalize()))
        }
    }
}

/// TODO: doc
pub mod md5_json {
    use md5::{Digest, Md5};

    use super::*;

    /// TODO: doc
    #[derive(Default, Clone)]
    pub struct Md5JsonTaskIdExtractor;

    impl<T: Serialize> TaskIdExtractor<T> for Md5JsonTaskIdExtractor {
        type Error = serde_json::Error;

        fn extract(&self, task: &T) -> Result<TaskId, Self::Error> {
            let bytes = serde_json::to_vec(task)?;
            let mut hasher = Md5::new();
            hasher.update(bytes);
            Ok(format!("{:x}", hasher.finalize()))
        }
    }
}
