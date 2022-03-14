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
pub use run::{start_deamon, start_mock};
pub use sealing::generator::generate_static_tree_d;
pub use sealing::processor::{
    external::sub::{run, run_c2, run_pc1, run_pc2, run_tree_d},
    Input, SnapProveReplicaUpdateInput, SnapReplicaUpdateInput,
};
pub use sealing::store;
pub use sealing::util as seal_util;
pub use util::task::block_on;
pub use watchdog::dones;

pub mod client;
pub mod logging;
pub mod sys;
