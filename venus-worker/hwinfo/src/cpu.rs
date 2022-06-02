use std::fmt::Display;

use hwloc2::{ObjectType, Topology, TopologyObject};
use strum::AsRefStr;

use crate::byte_string;

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

#[derive(Debug, AsRefStr)]
#[strum(serialize_all = "PascalCase")]
pub enum TopologyType {
    /// The typical root object type. A set of processors and memory with cache
    /// coherency.
    Machine,
    /// Physical package, what goes into a socket. In the physical meaning,
    Package { total_memory: u64 },
    /// A computation unit (may be shared by several logical processors).
    Core,
    /// Processing Unit, or (Logical) Processor.
    ///
    /// An execution unit (may share a core with some other logical
    /// processors, e.g. in the case of an SMT core). Objects of this kind
    /// are always reported and can thus be used as fallback when others are
    /// not.
    PU,
    /// Cache
    #[strum(disabled)]
    Cache { cache_type: CacheType, size: u64 },
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
    NUMANode,
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

pub struct TopologyNode {
    pub logical_index: u32,
    pub children: Vec<TopologyNode>,
    pub ty: TopologyType,
}

impl Display for TopologyNode {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match &self.ty {
            TopologyType::Cache { cache_type, size } => f.write_fmt(format_args!("{} ({})", cache_type.as_ref(), byte_string(*size, 0),)),
            TopologyType::Package { total_memory } => f.write_fmt(format_args!(
                "{} (total memory: {})",
                self.ty.as_ref(),
                byte_string(*total_memory, 2),
            )),
            _ => f.write_fmt(format_args!("{} #{}", self.ty.as_ref(), self.logical_index)),
        }
    }
}

/// load CPU Topology
pub fn load() -> Option<TopologyNode> {
    let topo = Topology::new()?;

    let root_obj = topo.object_at_root();
    assert!(root_obj.object_type() == ObjectType::Machine);
    Some(TopologyNode {
        logical_index: root_obj.logical_index(),
        children: load_recursive(root_obj),
        ty: TopologyType::Machine,
    })
}

fn load_recursive(parent: &TopologyObject) -> Vec<TopologyNode> {
    parent
        .children()
        .into_iter()
        .filter_map(|child_topo_obj| {
            child_topo_obj.try_into().ok().map(|topo_type| TopologyNode {
                logical_index: child_topo_obj.logical_index(),
                children: load_recursive(child_topo_obj),
                ty: topo_type,
            })
        })
        .collect()
}

pub struct Unsupport;

impl TryFrom<&TopologyObject> for TopologyType {
    type Error = Unsupport;

    fn try_from(hwloc2_topo_obj: &TopologyObject) -> Result<Self, Self::Error> {
        let get_cache_size = || hwloc2_topo_obj.cache_attributes().map(|attr| attr.size()).unwrap_or(0);
        Ok(match hwloc2_topo_obj.object_type() {
            ObjectType::Machine => TopologyType::Machine,
            ObjectType::Package => TopologyType::Package {
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
            ObjectType::NUMANode => TopologyType::NUMANode,
            ObjectType::Memcache => TopologyType::Memcache,
            ObjectType::Die => TopologyType::Die,

            _ => return Err(Unsupport),
        })
    }
}
