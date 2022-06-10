#[cfg(not(target_os = "linux"))]
pub use other::CtrlGroupDummy as CtrlGroup;

#[cfg(target_os = "linux")]
pub use linux::CtrlGroup;

#[cfg(not(target_os = "linux"))]
mod other {
    use anyhow::Result;

    use super::super::config;
    use crate::logging::warn;

    #[derive(Debug, PartialEq, Eq, PartialOrd, Ord)]
    pub struct CgroupPid(u64);

    impl From<u64> for CgroupPid {
        fn from(pid: u64) -> Self {
            Self(pid)
        }
    }

    pub struct CtrlGroupDummy {}

    impl CtrlGroupDummy {
        pub fn new(_name: &str, _cfg: &config::Cgroup) -> Result<Self> {
            warn!("{} does not support cgroups.", std::env::consts::OS);
            Ok(Self {})
        }

        pub fn add_task(&mut self, _pid: CgroupPid) -> Result<()> {
            Ok(())
        }

        #[allow(dead_code)]
        pub fn delete(&mut self) {}
    }
}

#[cfg(target_os = "linux")]
mod linux {
    use std::collections::HashSet;

    use anyhow::{Context, Result};
    use cgroups_rs::{hierarchies, Cgroup, CgroupPid, Controller, Subsystem};

    use super::super::config;
    use crate::logging::warn;

    const DEFAULT_CGROUP_GROUP_NAME: &str = "vc-worker";

    pub struct CtrlGroup {
        subsystems: Vec<Subsystem>,
    }

    impl CtrlGroup {
        pub fn new(name: &str, cfg: &config::Cgroup) -> Result<Self> {
            let group_name = cfg
                .group_name
                .as_ref()
                .cloned()
                .unwrap_or_else(|| DEFAULT_CGROUP_GROUP_NAME.to_owned());

            let mut wanted = HashSet::new();

            let cgname = format!("{}/{}", group_name, name);
            let cg = Cgroup::load(hierarchies::auto(), &cgname);
            let cgsubs = cg.subsystems();

            if let Some(cpuset) = cfg.cpuset.as_ref() {
                for (sidx, sub) in cgsubs.iter().enumerate() {
                    if let Subsystem::CpuSet(ctrl) = sub {
                        ctrl.create();
                        ctrl.set_cpus(cpuset).with_context(|| format!("set cpuset to {}", cpuset))?;
                        wanted.insert(sidx);
                        break;
                    }
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

        pub fn add_task(&mut self, pid: CgroupPid) -> Result<()> {
            for sub in self.subsystems.iter() {
                sub.to_controller()
                    .add_task(&pid)
                    .with_context(|| format!("subsystem {}", sub.controller_name()))?;
            }

            Ok(())
        }

        pub fn delete(&mut self) {
            for sub in self.subsystems.iter() {
                if let Err(e) = sub.to_controller().delete() {
                    warn!(system = sub.controller_name().as_str(), "subsystem delete failed: {:?}", e);
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
