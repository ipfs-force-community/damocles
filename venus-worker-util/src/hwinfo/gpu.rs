use rust_gpu_tools::Device;
use strum::AsRefStr;

/// GPU Vendor
#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq, AsRefStr)]
#[strum(serialize_all = "UPPERCASE")]
pub enum Vendor {
    /// GPU by AMD.
    Amd,
    /// GPU by NVIDIA.
    Nvidia,
}

/// Information about the GPU
pub struct GPUInfo {
    /// The GPU name
    pub name: String,
    /// The GPU vendor
    pub vendor: Vendor,
    /// The GPU memory size, in bytes
    pub memory: u64,
}

/// Load GPU information
pub fn load() -> Vec<GPUInfo> {
    Device::all()
        .iter()
        .map(|dev| GPUInfo {
            name: dev.name(),
            vendor: dev.vendor().into(),
            memory: dev.memory(),
        })
        .collect()
}

impl From<rust_gpu_tools::Vendor> for Vendor {
    fn from(rust_gpu_tools_vendor: rust_gpu_tools::Vendor) -> Self {
        match rust_gpu_tools_vendor {
            rust_gpu_tools::Vendor::Amd => Vendor::Amd,
            rust_gpu_tools::Vendor::Nvidia => Vendor::Nvidia,
        }
    }
}
