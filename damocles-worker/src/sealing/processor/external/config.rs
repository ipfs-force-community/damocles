//! config structs for external processors

use std::collections::HashMap;
use std::os::raw::c_int;
use std::time::Duration;

use serde::{Deserialize, Serialize};

/// configurations for cgroup used in processor
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Cgroup {
    pub group_name: Option<String>,

    /// the cpuset which will be applied onto the control group of the external processor
    pub cpuset: Option<String>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Ext {
    /// binary path
    pub bin: Option<String>,

    pub args: Option<Vec<String>>,

    #[serde(default = "default_stable_wait")]
    #[serde(with = "humantime_serde")]
    /// wait duration before processor get ready
    pub stable_wait: Duration,

    /// cgroup params for the sub-process of the external processor
    pub cgroup: Option<Cgroup>,

    /// env pairs for the sub-process of the external processor
    pub envs: Option<HashMap<String, String>>,

    /// preferred numa node number
    pub numa_preferred: Option<c_int>,

    /// concurrent limit
    pub concurrent: Option<usize>,

    pub locks: Option<Vec<String>>,

    /// Whether to restart the child process automatically
    /// after the child process exited
    #[serde(default = "default_auto_restart")]
    pub auto_restart: bool,

    /// Whether to inherit the environment variables from the worker daemon process
    #[serde(default = "default_inherit_envs")]
    pub inherit_envs: bool,
}

#[inline]
fn default_inherit_envs() -> bool {
    true
}

#[inline]
fn default_auto_restart() -> bool {
    true
}

#[inline]
fn default_stable_wait() -> Duration {
    Duration::from_secs(5)
}
