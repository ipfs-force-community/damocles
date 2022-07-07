use std::env::var;
use std::os::raw::{c_int, c_uint, c_ulong};

use lazy_static::lazy_static;
use tracing::{info, warn};

/// env key for preferred numa node
pub const ENV_NUMA_PREFERRED: &str = "VENUS_WORKER_NUMA_PREFERRED";

lazy_static! {
    static ref NUMA_AVAILABLE: bool = unsafe { numa_available() >= 0 };
}

#[link(name = "numa", kind = "dylib")]
extern "C" {
    /// Check if NUMA support is enabled. Returns -1 if not enabled, in which case other functions will undefined
    fn numa_available() -> c_int;
    fn numa_set_preferred(node: c_int);
    fn numa_max_node() -> c_int;

    /// returns a bitmask of a size equal to the kernel's node mask (kernel type nodemask_t).
    /// In other words, large enough to represent MAX_NUMNODES nodes.
    fn numa_allocate_nodemask() -> *mut NumaBitmask;

    /// Returns a bitmask of a size equal to the kernel's cpu mask (kernel type cpumask_t).
    /// In other words, large enough to represent NR_CPUS cpus.
    fn numa_allocate_cpumask() -> *mut NumaBitmask;

    /// Deallocates the memory of both the bitmask structure pointed to by bmp and the bit mask.
    /// It is an error to attempt to free this bitmask twice.
    fn numa_bitmask_free(bmp: *mut NumaBitmask);

    /// Sets a specified bit in a bit mask to 1.
    /// Nothing is done if n is greater than the size of the bitmask (and no error is returned).
    /// The value of bmp is always returned.
    fn numa_bitmask_setbit(bmp: *mut NumaBitmask, n: c_uint) -> *mut NumaBitmask;

    /// Binds the current task and its children to the nodes specified in nodemask.
    /// They will only run on the CPUs of the specified nodes and only be able to allocate memory from them.
    fn numa_bind(nodemask: *mut NumaBitmask);
}

#[derive(Debug)]
pub struct NumaNotAvailable;

pub struct Numa;

impl Numa {
    pub fn new() -> Result<Self, NumaNotAvailable> {
        if *NUMA_AVAILABLE {
            Ok(Self)
        } else {
            Err(NumaNotAvailable)
        }
    }

    pub fn alloc_nodemask(&self) -> Bitmask {
        Bitmask::alloc_nodemask()
    }

    pub fn alloc_cpumask(&self) -> Bitmask {
        Bitmask::alloc_cpumask()
    }

    pub fn bind(&self, node: u32) -> Result<(), u64> {
        bind(node)
    }

    pub fn bind_with_nodemask(&self, nodemask: &mut Bitmask) {
        bind_with_nodemask(nodemask)
    }
}

#[repr(C)]
struct NumaBitmask {
    /// number of bits in the map
    size: c_ulong,
    maskp: *mut c_ulong,
}

pub struct Bitmask {
    inner: *mut NumaBitmask,
}

impl Bitmask {
    fn alloc_nodemask() -> Self {
        Self {
            inner: unsafe { numa_allocate_nodemask() },
        }
    }

    fn alloc_cpumask() -> Self {
        Self {
            inner: unsafe { numa_allocate_cpumask() },
        }
    }

    pub fn set_bit(&mut self, bit: u32) {
        unsafe {
            numa_bitmask_setbit(self.inner, bit as c_uint);
        }
    }

    pub fn size(&self) -> u64 {
        unsafe { (*self.inner).size as u64 }
    }
}

impl Drop for Bitmask {
    fn drop(&mut self) {
        unsafe { numa_bitmask_free(self.inner) }
    }
}

fn bind(node: u32) -> Result<(), u64> {
    let mut nodemask = Bitmask::alloc_nodemask();
    if node as u64 >= nodemask.size() {
        // out of bounds
        return Err(nodemask.size());
    }
    nodemask.set_bit(node);
    bind_with_nodemask(&mut nodemask);
    Ok(())
}

fn bind_with_nodemask(nodemask: &mut Bitmask) {
    unsafe {
        numa_bind(nodemask.inner);
    }
}

/// set numa policy to preferred for current process
pub fn set_preferred(node: c_int) {
    unsafe {
        if !*NUMA_AVAILABLE {
            warn!("numa is not available");
            return;
        }

        let max = numa_max_node();
        if node > max {
            warn!(max, "node number {} is invalid", node);
            return;
        }

        numa_set_preferred(node);
        info!("set numa policy to preferred({})", node);
    }
}

/// try to extract and set preferred node  from env
pub fn try_set_preferred() {
    let s = match var(ENV_NUMA_PREFERRED) {
        Ok(v) => v,
        _ => return,
    };

    let node = match s.parse() {
        Ok(n) => n,
        _ => {
            warn!("invalid numa node string {}", s);
            return;
        }
    };

    set_preferred(node);
}
