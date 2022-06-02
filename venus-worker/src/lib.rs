#![deny(missing_docs)]
//! venus-worker is used to seal sectors based on local resources

pub(crate) mod config;
pub(crate) mod infra;
pub(crate) mod metadb;
pub(crate) mod rpc;
pub(crate) mod sealing;
pub(crate) mod signal;
pub(crate) mod types;
pub(crate) mod watchdog;

mod run;
mod util;

pub use config::Config;
pub use infra::objstore;
pub use run::start_deamon;
pub use sealing::processor::{create_tree_d, RegisteredSealProof, SnapEncodeInput, SnapProveInput};
pub use sealing::store;
pub use sealing::util as seal_util;
pub use types::SealProof;
pub use util::task::block_on;
pub use watchdog::dones;

pub mod client;
pub mod logging;
pub mod sys;
