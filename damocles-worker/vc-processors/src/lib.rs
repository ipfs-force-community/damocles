#![deny(missing_docs)]
//! vc-processors contains the types and builtin external processors for
//! [damocles/damocles-worker](https://github.com/ipfs-force-community/damocles/tree/main/damocles-worker).
//!
//! This crate aims at providing an easy-to-use, buttery-included framework for third-party
//! developers to implement customized external processors.
//!
//! This crate could be used to:
//! 1. interact with builtin external processors in damocles-worker.
//! 2. wrap close-source processors for builtin tasks in damocles-worker.
//! 3. implement any customized external processors for other usecases.
//!
//! The [examples](https://github.com/ipfs-force-community/damocles/tree/main/damocles-worker/vc-processors/examples) show more details about the usages.
//!

pub mod core;
pub mod sys;

#[cfg(feature = "fil-proofs")]
#[allow(missing_docs)]
pub mod fil_proofs;

#[cfg(any(feature = "builtin-tasks", feature = "builtin-processors"))]
pub mod builtin;

#[cfg(feature = "b64serde")]
#[allow(missing_docs)]
pub mod b64serde;
