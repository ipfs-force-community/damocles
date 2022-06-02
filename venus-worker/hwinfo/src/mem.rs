use sysinfo::{RefreshKind, System, SystemExt};

/// The memory infomation
pub struct Mem {
    /// The RAM size in KB.
    pub total_mem: u64,
    /// The amount of used RAM in KB.
    pub used_mem: u64,
    /// The SWAP size in KB.
    pub total_swap: u64,
    /// The amount of used SWAP in KB.
    pub used_swap: u64,
}

/// Load memory infomation
pub fn load() -> Mem {
    let sys = System::new_with_specifics(RefreshKind::new().with_memory());
    Mem {
        total_mem: sys.total_memory(),
        used_mem: sys.used_memory(),
        total_swap: sys.total_swap(),
        used_swap: sys.used_swap(),
    }
}
