//! This module provides cgroup helpers
//!

use std::ffi::OsString;

pub use imp::*;
use tokio_transports::rw::pipe;

/// ENV name of cgroup name that we want to load
pub const ENV_CGROUP_NAME: &str = "VC_CG_NAME";
/// ENV name of cpuset that we want to load
pub const ENV_CGROUP_CPUSET: &str = "VC_CG_CPUSET";

/// set cpuset
pub fn pipe_set_cpuset(cmd: pipe::Command, cgname: impl Into<OsString>, cpuset: impl Into<OsString>) -> pipe::Command {
    cmd.envs([(ENV_CGROUP_NAME, cgname.into()), (ENV_CGROUP_CPUSET, cpuset.into())])
}

#[cfg(not(target_os = "linux"))]
mod imp {
    use anyhow::Result;

    /// try load cgroup from env
    pub fn try_set_from_env() -> CtrlGroup {
        CtrlGroup::new("", "").expect("never fail")
    }

    /// CgroupPid represents a pid
    #[derive(Debug, PartialEq, Eq, PartialOrd, Ord)]
    pub struct CgroupPid(u64);

    impl From<u64> for CgroupPid {
        fn from(pid: u64) -> Self {
            Self(pid)
        }
    }

    /// An Cgroup type
    pub struct CtrlGroup {}

    impl CtrlGroup {
        /// Creates a new CtrlGroup with cgroup name `_cgname` and cpuset `_cpuset`.
        pub fn new(_cgname: impl AsRef<str>, _cpuset: impl AsRef<str>) -> Result<Self> {
            tracing::warn!("{} does not support cgroups.", std::env::consts::OS);
            Ok(Self {})
        }

        /// Attach a task to this controller.
        pub fn add_task(&mut self, _pid: CgroupPid) -> Result<()> {
            Ok(())
        }

        /// Delete the controller.
        #[allow(dead_code)]
        pub fn delete(&mut self) {}
    }
}

#[cfg(target_os = "linux")]
mod imp {
    use std::collections::HashSet;

    use anyhow::{Context, Result};
    use cgroups_rs::{hierarchies, Cgroup, CgroupPid, Controller, Subsystem};

    use super::{ENV_CGROUP_CPUSET, ENV_CGROUP_NAME};

    /// try load cgroup from env
    pub fn try_set_from_env() -> CtrlGroup {
        use std::env::var;

        match (var(ENV_CGROUP_NAME), var(ENV_CGROUP_CPUSET)) {
            (Ok(cgname), Ok(cpuset)) => match CtrlGroup::new(&cgname, &cpuset) {
                Ok(mut cg) => {
                    let pid = std::process::id() as u64;
                    match cg.add_task(pid.into()) {
                        Ok(_) => {
                            tracing::info!(pid = pid, group = cgname.as_str(), "add into cgroup");
                        }
                        Err(e) => {
                            tracing::error!(pid = pid, group = cgname.as_str(), err=?e, "failed to add cgroup");
                        }
                    }
                    cg
                }
                Err(e) => {
                    tracing::error!(err=?e, "failed to load cgroup cpuset from env. cgname: {}, cpuset: {}", cgname.as_str(), cpuset.as_str());
                    CtrlGroup::empty()
                }
            },
            _ => CtrlGroup::empty(),
        }
    }

    /// An Cgroup type
    pub struct CtrlGroup {
        subsystems: Vec<Subsystem>,
    }

    impl CtrlGroup {
        /// Returns an empty `CtrlGroup`
        pub fn empty() -> Self {
            Self { subsystems: vec![] }
        }

        /// Creates a new CtrlGroup with cgroup name `cgname` and cpuset.
        pub fn new(cgname: impl AsRef<str>, cpuset: impl AsRef<str>) -> Result<Self> {
            let mut wanted = HashSet::new();

            let cg = Cgroup::load(hierarchies::auto(), cgname.as_ref());
            let cgsubs = cg.subsystems();

            for (sidx, sub) in cgsubs.iter().enumerate() {
                if let Subsystem::CpuSet(ctrl) = sub {
                    ctrl.create();
                    ctrl.set_cpus(cpuset.as_ref())
                        .with_context(|| format!("set cpuset to {}", cpuset.as_ref()))?;
                    wanted.insert(sidx);
                    break;
                }
            }

            let mut subsystems = Vec::with_capacity(cgsubs.len());
            for (sidx, sub) in cgsubs.iter().enumerate() {
                if wanted.contains(&sidx) {
                    subsystems.push(sub.clone());
                }
            }

            Ok(CtrlGroup { subsystems })
        }

        /// Attach a task to this controller.
        pub fn add_task(&mut self, pid: CgroupPid) -> Result<()> {
            for sub in self.subsystems.iter() {
                sub.to_controller()
                    .add_task(&pid)
                    .with_context(|| format!("subsystem {}", sub.controller_name()))?;
            }

            Ok(())
        }

        /// Delete the controller.
        pub fn delete(&mut self) {
            for sub in self.subsystems.iter() {
                if let Err(e) = sub.to_controller().delete() {
                    tracing::warn!(system = sub.controller_name().as_str(), "subsystem delete failed: {:?}", e);
                };
            }
        }
    }

    impl Drop for CtrlGroup {
        fn drop(&mut self) {
            self.delete();
        }
    }
}
