//! helpers for numa control

use std::env::var;
use std::os::raw::c_int;

use crate::logging::{info, warn};

/// env key for preferred numa node
pub const ENV_NUMA_PREFERRED: &str = "VENUS_WORKER_NUMA_PREFERRED";

#[link(name = "numa", kind = "dylib")]
extern "C" {
    fn numa_available() -> c_int;
    fn numa_set_preferred(node: c_int);
    fn numa_max_node() -> c_int;
}

/// set numa policy to preferred for currrent process
pub fn set_preferred(node: c_int) {
    unsafe {
        if numa_available() < 0 {
            warn!("numa is not available");
            return;
        }

        let max = numa_max_node();
        if node > max {
            warn!(max, "node number {} is invalid", node);
            return;
        }

        numa_set_preferred(node);
        info!("set numa policy to preferred({})", node);
    }
}

/// try to extract and set preferred node  from env
pub fn try_set_preferred() {
    let s = match var(ENV_NUMA_PREFERRED) {
        Ok(v) => v,
        _ => return,
    };

    let node = match s.parse() {
        Ok(n) => n,
        _ => {
            warn!("invalid numa node string {}", s);
            return;
        }
    };

    set_preferred(node);
}
