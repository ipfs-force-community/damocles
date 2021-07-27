//! config structs for external processors

use std::collections::HashMap;
use std::time::Duration;

use serde::{Deserialize, Serialize};

/// default stable wait duration
pub const EXT_STABLE_WAIT: Duration = Duration::from_secs(5);

/// configurations for cgroup used in processor
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Cgroup {
    /// the cpuset which will be applied onto the control group of the external processor
    pub cpuset: Option<String>,
}

/// configurations for each single processor used in sealing
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Ext {
    /// enable external processor
    pub external: bool,

    /// path for binary of external processor
    pub bin: Option<String>,

    #[serde(default)]
    #[serde(with = "humantime_serde")]
    /// wait duration before processor get ready
    pub stable_wait: Option<Duration>,

    /// cgroup params for the sub-process of the external processor
    pub cgroup: Option<Cgroup>,

    /// env pairs for the sub-process of the external processor
    pub envs: Option<HashMap<String, String>>,
}
