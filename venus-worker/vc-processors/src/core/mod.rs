//! This module provides the most important types and abstractions
//!

use std::fmt::Debug;

use serde::{de::DeserializeOwned, Serialize};

pub mod ext;

/// Task of a specific stage, with Output & Error defined
pub trait Task
where
    Self: Serialize + DeserializeOwned + Debug + Send + Sync + 'static,
    Self::Output: Serialize + DeserializeOwned + Debug + Send + Sync + 'static,
    Self::Error: Debug,
{
    /// The stage name.
    const STAGE: &'static str;

    /// The output type
    type Output;

    /// The error type
    type Error;

    /// Execute the task
    fn exec(self) -> Result<Self::Output, Self::Error>;
}

/// Processor of a specific task type
pub trait Processor<T>
where
    Self: Send + Sync,
    T: Task,
{
    /// Handle the given task.
    fn handle(&self, task: T) -> Result<T::Output, T::Error> {
        task.exec()
    }
}
