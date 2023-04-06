//! This module provides the most important types and abstractions
//!

use std::fmt::Debug;

use anyhow::Result;
use serde::{de::DeserializeOwned, Serialize};

pub mod ext;

/// Task of a specific stage, with Output & Error defined
pub trait Task
where
    Self: Serialize + DeserializeOwned + Debug + Send + Sync + 'static,
    Self::Output: Serialize + DeserializeOwned + Debug + Send + Sync + 'static,
{
    /// The stage name.
    const STAGE: &'static str;

    /// The output type
    type Output;
}

/// Processor of a specific task type
pub trait Processor<T: Task>
where
    Self: Send + Sync,
{
    /// Process the given task.
    fn process(&self, task: T) -> Result<<T as Task>::Output>;
}
