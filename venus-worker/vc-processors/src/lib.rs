#![deny(missing_docs)]
//! vc-processors contains the types and builtin external processors for
//! [venus-cluster/venus-worker](https://github.com/ipfs-force-community/venus-cluster/tree/main/venus-worker).
//!
//! This crate aims at providing an easy-to-use, buttery-inlcuded framework for third-party
//! developers to implement customized external processors.
//!
//! This crate could be used to:
//! 1. interact with builtin external processors in venus-worker.
//! 2. wrap close-source processors for builtin tasks in venus-worker.
//! 3. implement any customized external processors for other usecases.

pub mod core;
pub(crate) mod sys;

#[cfg(feature = "fil-proofs")]
#[allow(missing_docs)]
pub mod fil_proofs;

#[cfg(any(feature = "builtin-tasks", feature = "builtin-processors"))]
pub mod builtin;
