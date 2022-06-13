use sysinfo::{RefreshKind, System, SystemExt};

/// Information about the Memory
pub struct Mem {
    /// The RAM size in bytes.
    pub total_mem: u64,
    /// The amount of used RAM in bytes.
    pub used_mem: u64,
    /// The SWAP size in bytes.
    pub total_swap: u64,
    /// The amount of used SWAP in bytes.
    pub used_swap: u64,
}

/// Load memory infomation
pub fn load() -> Mem {
    let sys = System::new_with_specifics(RefreshKind::new().with_memory());
    Mem {
        total_mem: sys.total_memory() * 1024,
        used_mem: sys.used_memory() * 1024,
        total_swap: sys.total_swap() * 1024,
        used_swap: sys.used_swap() * 1024,
    }
}
