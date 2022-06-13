use strum::AsRefStr;
use sysinfo::{DiskExt, RefreshKind, System, SystemExt};

#[derive(Debug, PartialEq, Clone, Copy, AsRefStr)]
#[strum(serialize_all = "UPPERCASE")]
pub enum DiskType {
    /// HDD type.
    HDD,
    /// SSD type.
    SSD,
}

/// Information about the disk
pub struct Disk {
    pub disk_type: DiskType,
    pub device_name: String,
    pub filesystem: String,
    pub total_space: u64,
    pub available_space: u64,
    pub is_removable: bool,
}

/// load disks infomation
pub fn load() -> Vec<Disk> {
    let sys = System::new_with_specifics(RefreshKind::new().with_disks_list());
    sys.disks()
        .iter()
        .filter_map(|disk| {
            Some(Disk {
                disk_type: match disk.type_() {
                    sysinfo::DiskType::HDD => DiskType::HDD,
                    sysinfo::DiskType::SSD => DiskType::SSD,
                    sysinfo::DiskType::Unknown(_) => return None,
                },
                device_name: disk.name().to_string_lossy().to_string(),
                filesystem: String::from_utf8_lossy(disk.file_system()).to_string(),
                total_space: disk.total_space(),
                available_space: disk.available_space(),
                is_removable: disk.is_removable(),
            })
        })
        .collect()
}
