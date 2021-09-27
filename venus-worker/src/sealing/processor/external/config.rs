//! config structs for external processors

use std::collections::HashMap;
use std::os::raw::c_int;
use std::time::Duration;

use serde::{Deserialize, Serialize};

/// default stable wait duration
pub const EXT_STABLE_WAIT: Duration = Duration::from_secs(5);

/// configurations for cgroup used in processor
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Cgroup {
    pub group_name: Option<String>,

    /// the cpuset which will be applied onto the control group of the external processor
    pub cpuset: Option<String>,
}

/// configurations for each single processor used in sealing
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Ext {
    /// enable external processor
    pub external: bool,

    /// options for each sub processor
    pub subs: Option<Vec<ExtSub>>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct ExtSub {
    /// binary path
    pub bin: Option<String>,

    #[serde(default)]
    #[serde(with = "humantime_serde")]
    /// wait duration before processor get ready
    pub stable_wait: Option<Duration>,

    /// cgroup params for the sub-process of the external processor
    pub cgroup: Option<Cgroup>,

    /// env pairs for the sub-process of the external processor
    pub envs: Option<HashMap<String, String>>,

    /// preferred numa node number
    pub numa_preferred: Option<c_int>,
}
