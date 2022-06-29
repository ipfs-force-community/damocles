use std::fmt::Display;

use hwloc2::{get_api_version, ObjectType, Topology, TopologyObject, TopologyObjectInfo};
use strum::AsRefStr;

use super::byte_string;

/// Type of CPU Cache
#[derive(Debug, AsRefStr)]
#[strum(serialize_all = "PascalCase")]
pub enum CacheType {
    /// Data cache
    ///
    /// Level 1 Data (or Unified) Cache.
    L1,
    /// Data cache
    ///
    /// Level 2 Data (or Unified) Cache.
    L2,
    /// Data cache
    ///
    /// Level 3 Data (or Unified) Cache.
    L3,
    /// Data cache
    ///
    /// Level 4 Data (or Unified) Cache.
    L4,
    /// Data cache
    ///
    /// Level 5 Data (or Unified) Cache.
    L5,
}

/// Type of CPU topology
#[derive(Debug, AsRefStr)]
#[strum(serialize_all = "PascalCase")]
pub enum TopologyType {
    /// The typical root object type. A set of processors and memory with cache
    /// coherency.
    Machine {
        /// CPU model of this object if present
        cpu_model: Option<String>,
        /// The memory attributes of the object, in bytes.
        total_memory: u64,
    },
    /// Physical package, what goes into a socket. In the physical meaning,
    Package {
        /// CPU model of this object if present
        cpu_model: Option<String>,
        /// The memory attributes of the object, in bytes.
        total_memory: u64,
    },
    /// A computation unit (may be shared by several logical processors).
    Core,
    /// Processing Unit, or (Logical) Processor.
    ///
    /// An execution unit (may share a core with some other logical
    /// processors, e.g. in the case of an SMT core). Objects of this kind
    /// are always reported and can thus be used as fallback when others are
    /// not.
    #[strum(serialize = "PU")]
    PU,
    /// Cache
    #[strum(disabled)]
    Cache {
        /// type of the CPU cache
        cache_type: CacheType,
        /// cache size in bytes
        size: u64,
    },
    /// Group objects.
    ///
    /// Objects which do not fit in the above but are detected by hwloc and
    /// are useful to take into account for affinity. For instance, some
    /// operating systems expose their arbitrary processors aggregation this
    /// way. And hwloc may insert such objects to group NUMA nodes according
    /// to their distances.
    ///
    /// These objects are ignored when they do not bring any structure.
    Group,
    /// A set of processors around memory which the processors can directly
    /// access.
    #[strum(serialize = "NUMANode")]
    NUMANode {
        /// The memory attributes of the object, in bytes.
        total_memory: u64,
    },
    /// Memory-side cache
    ///
    /// Memory-side cache (filtered out by default).
    ///
    /// A cache in front of a specific NUMA node.
    ///
    /// Memory objects are not listed in the main children list, but rather in the dedicated Memory
    /// children list.
    ///
    /// Memory-side cache have a special depth ::HWLOC_TYPE_DEPTH_MEMCACHE instead of a normal
    /// depth just like other objects in the main tree.
    Memcache,
    /// Die within a physical package.
    ///
    /// A subpart of the physical package, that contains multiple cores.
    Die,
}

/// Represents the type of a topology Node.
pub struct TopologyNode {
    /// Horizontal index in the whole list of similar objects, hence guaranteed
    /// unique across the entire machine.
    pub logical_index: u32,
    /// All direct children of this object.
    pub children: Vec<TopologyNode>,
    /// Type of the CPU topology
    pub ty: TopologyType,
}

impl Display for TopologyNode {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match &self.ty {
            TopologyType::Cache { cache_type, size } => f.write_fmt(format_args!(
                "{} (#{} {})",
                cache_type.as_ref(),
                self.logical_index,
                byte_string(*size, 0),
            )),
            TopologyType::Machine {
                cpu_model,
                total_memory,
            }
            | TopologyType::Package {
                cpu_model,
                total_memory,
            } => match cpu_model {
                Some(m) => f.write_fmt(format_args!(
                    "{} ({}) ({})",
                    self.ty.as_ref(),
                    byte_string(*total_memory, 2),
                    m
                )),
                None => f.write_fmt(format_args!(
                    "{} ({})",
                    self.ty.as_ref(),
                    byte_string(*total_memory, 2)
                )),
            },
            TopologyType::NUMANode { total_memory } => f.write_fmt(format_args!(
                "{} (#{} {})",
                self.ty.as_ref(),
                self.logical_index,
                byte_string(*total_memory, 2)
            )),
            _ => f.write_fmt(format_args!("{} #{}", self.ty.as_ref(), self.logical_index)),
        }
    }
}

/// Check hwloc version, make sure the hwloc version is at least 2.0.0
fn check_hwloc_version() -> bool {
    let hwloc_api_version = get_api_version();
    // (X<<16)+(Y<<8)+Z represents X.Y.Z
    let hwloc_x_version = hwloc_api_version >> 16;
    hwloc_x_version >= 2
}

/// load CPU Topology
pub fn load() -> Option<TopologyNode> {
    if !check_hwloc_version() {
        return None;
    }
    let topo = Topology::new()?;
    let root_obj = topo.object_at_root();
    Some(TopologyNode {
        logical_index: root_obj.logical_index(),
        children: load_recursive(root_obj),
        ty: root_obj.try_into().ok()?,
    })
}

fn load_recursive(parent: &TopologyObject) -> Vec<TopologyNode> {
    let mut nodes = Vec::new();

    // traversal numa node
    // see: https://www.open-mpi.org/projects/hwloc/doc/v2.3.0/a00360.php
    for memory_child_obj in parent.memory_children() {
        if let Ok(ty) = memory_child_obj.try_into() {
            nodes.push(TopologyNode {
                logical_index: memory_child_obj.logical_index(),
                children: Vec::new(),
                ty,
            });
        }
    }

    nodes.extend(parent.children().into_iter().filter_map(|child_topo_obj| {
        child_topo_obj
            .try_into()
            .ok()
            .map(|topo_type| TopologyNode {
                logical_index: child_topo_obj.logical_index(),
                children: load_recursive(child_topo_obj),
                ty: topo_type,
            })
    }));
    nodes
}

/// Unsupported Error
pub struct Unsupported;

impl TryFrom<&TopologyObject> for TopologyType {
    type Error = Unsupported;

    fn try_from(hwloc2_topo_obj: &TopologyObject) -> Result<Self, Self::Error> {
        let get_cache_size = || {
            hwloc2_topo_obj
                .cache_attributes()
                .map(|attr| attr.size())
                .unwrap_or(0)
        };
        let find_info_value = |key| {
            hwloc2_topo_obj
                .infos()
                .iter()
                .find(|info| info.name().to_str() == Ok(key))
                .map(|info| info.value().to_string_lossy().trim().to_string())
        };
        Ok(match hwloc2_topo_obj.object_type() {
            ObjectType::Machine => TopologyType::Machine {
                cpu_model: find_info_value(TopologyObjectInfo::NAME_OF_CPU_MODEL),
                total_memory: hwloc2_topo_obj.total_memory(),
            },
            ObjectType::Package => TopologyType::Package {
                cpu_model: find_info_value(TopologyObjectInfo::NAME_OF_CPU_MODEL),
                total_memory: hwloc2_topo_obj.total_memory(),
            },
            ObjectType::Core => TopologyType::Core,
            ObjectType::PU => TopologyType::PU,
            ObjectType::L1Cache => TopologyType::Cache {
                cache_type: CacheType::L1,
                size: get_cache_size(),
            },
            ObjectType::L2Cache => TopologyType::Cache {
                cache_type: CacheType::L2,
                size: get_cache_size(),
            },
            ObjectType::L3Cache => TopologyType::Cache {
                cache_type: CacheType::L3,
                size: get_cache_size(),
            },
            ObjectType::L4Cache => TopologyType::Cache {
                cache_type: CacheType::L4,
                size: get_cache_size(),
            },
            ObjectType::L5Cache => TopologyType::Cache {
                cache_type: CacheType::L5,
                size: get_cache_size(),
            },
            ObjectType::Group => TopologyType::Group,
            ObjectType::NUMANode => TopologyType::NUMANode {
                total_memory: hwloc2_topo_obj.total_memory(),
            },
            ObjectType::Memcache => TopologyType::Memcache,
            ObjectType::Die => TopologyType::Die,

            _ => return Err(Unsupported),
        })
    }
}
