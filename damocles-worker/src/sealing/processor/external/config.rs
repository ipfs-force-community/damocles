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

    /// the weight of this external proceessor
    ///
    /// the probability of each external proceessor being selected is `weight / s`,
    /// where `s` is the sum of the `weight' of all ext processor with the same stage_name.
    #[serde(default = "default_weight")]
    pub weight: u16,

    /// Whether to restart the child process automatically
    /// after the child process exited
    #[serde(default = "default_auto_restart")]
    pub auto_restart: bool,
}

#[inline]
fn default_weight() -> u16 {
    1
}

#[inline]
fn default_auto_restart() -> bool {
    true
}

#[inline]
fn default_stable_wait() -> Duration {
    Duration::from_secs(5)
}
