//! This module provides cgroup helpers
//!

pub use imp::*;

/// ENV name of cgroup name that we want to load
pub const ENV_CGROUP_NAME: &str = "VC_CG_NAME";
/// ENV name of cpuset that we want to load
pub const ENV_CGROUP_CPUSET: &str = "VC_CG_CPUSET";

#[cfg(not(target_os = "linux"))]
mod imp {
    use anyhow::Result;

    /// try load cgroup from env
    pub fn try_load_from_env() -> CtrlGroup {
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
        pub fn new(
            _cgname: impl AsRef<str>,
            _cpuset: impl AsRef<str>,
        ) -> Result<Self> {
            tracing::warn!(
                "{} does not support cgroups.",
                std::env::consts::OS
            );
            Ok(Self {})
        }

        /// Attach a task to this controller.
        pub fn add_task_by_tgid(&mut self, _tgid: CgroupPid) -> Result<()> {
            Ok(())
        }

        /// Delete the controller.
        #[allow(dead_code)]
        pub fn delete(&mut self) {}
    }
}

#[cfg(target_os = "linux")]
mod imp {
    use anyhow::{Context, Result};
    use cgroups_rs::{
        cpuset::CpuSetController, hierarchies, Cgroup, CgroupPid,
    };

    use super::{ENV_CGROUP_CPUSET, ENV_CGROUP_NAME};

    /// try load cgroup from env
    pub fn try_load_from_env() -> CtrlGroup {
        use std::env::var;

        match (var(ENV_CGROUP_NAME), var(ENV_CGROUP_CPUSET)) {
            (Ok(cgname), Ok(cpuset)) => {
                match CtrlGroup::new(&cgname, &cpuset) {
                    Ok(mut cg) => {
                        let tgid =
                            libc::pid_t::from(nix::unistd::getpid()) as u64;
                        match cg.add_task_by_tgid(tgid.into()) {
                            Ok(_) => {
                                tracing::info!(
                                    tgid = tgid,
                                    group = cgname.as_str(),
                                    cpuset = cpuset.as_str(),
                                    "add into cgroup"
                                );
                            }
                            Err(e) => {
                                tracing::error!(tgid = tgid, group = cgname.as_str(), err=?e, "failed to add cgroup");
                            }
                        }
                        cg
                    }
                    Err(e) => {
                        tracing::error!(err=?e, "failed to load cgroup cpuset from env. cgname: {}, cpuset: {}", cgname.as_str(), cpuset.as_str());
                        CtrlGroup::empty()
                    }
                }
            }
            _ => CtrlGroup::empty(),
        }
    }

    /// An Cgroup type
    pub struct CtrlGroup {
        cg: Cgroup,
    }

    impl CtrlGroup {
        /// Returns an empty `CtrlGroup`
        pub fn empty() -> Self {
            Self {
                cg: Default::default(),
            }
        }

        /// Creates a new CtrlGroup with cgroup name `cgname` and cpuset.
        pub fn new(
            cgname: impl AsRef<str>,
            cpus: impl AsRef<str>,
        ) -> Result<Self> {
            let cg = Cgroup::new(hierarchies::auto(), cgname.as_ref())?;
            let cpuset = cg
                .controller_of::<CpuSetController>()
                .expect("No cpu controller attached!");

            cpuset.set_cpus(cpus.as_ref())?;
            Ok(CtrlGroup { cg })
        }

        /// Attach a task to this controller.
        pub fn add_task_by_tgid(&mut self, tgid: CgroupPid) -> Result<()> {
            self.cg.add_task_by_tgid(tgid).context("add task to cgroup")
        }

        /// Delete the controller.
        pub fn delete(&mut self) {
            for task in self.cg.tasks() {
                let _ = self.cg.remove_task_by_tgid(task);
            }
            if let Err(e) = self.cg.delete() {
                tracing::warn!("subsystem delete failed: {:?}", e);
            }
        }
    }

    impl Drop for CtrlGroup {
        fn drop(&mut self) {
            self.delete();
        }
    }
}
