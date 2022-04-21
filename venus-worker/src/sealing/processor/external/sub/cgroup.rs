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
        let group_name = cfg.group_name.as_ref().cloned().unwrap_or(DEFAULT_CGROUP_GROUP_NAME.to_owned());

        let mut wanted = HashSet::new();

        let cgname = format!("{}/{}", group_name, name);
        let cg = Cgroup::load(hierarchies::auto(), &cgname);
        let cgsubs = cg.subsystems();

        if let Some(cpuset) = cfg.cpuset.as_ref() {
            for (sidx, sub) in cgsubs.iter().enumerate() {
                match sub {
                    Subsystem::CpuSet(ctrl) => {
                        ctrl.create();
                        ctrl.set_cpus(cpuset).with_context(|| format!("set cpuset to {}", cpuset))?;
                        wanted.insert(sidx);
                        break;
                    }

                    _ => {}
                }
            }
        }

        let mut subsystems = Vec::with_capacity(cgsubs.len());
        for (sidx, sub) in cgsubs.into_iter().enumerate() {
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
