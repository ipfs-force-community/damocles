use std::path::PathBuf;

use crate::rpc::sealer::SectorID;

pub fn sector_path(sector_id: &SectorID) -> String {
    format!("s-t0{}-{}", sector_id.miner, sector_id.number)
}

pub fn sealed_file(sector_id: &SectorID) -> PathBuf {
    PathBuf::from("sealed").join(sector_path(sector_id))
}

pub fn cache_dir(sector_id: &SectorID) -> PathBuf {
    PathBuf::from("cache").join(sector_path(sector_id))
}

pub fn update_file(sector_id: &SectorID) -> PathBuf {
    PathBuf::from("update").join(sector_path(sector_id))
}

pub fn update_cache_dir(sector_id: &SectorID) -> PathBuf {
    PathBuf::from("update-cache").join(sector_path(sector_id))
}

pub fn pc2_running_file(sector_id: &SectorID) -> PathBuf {
    cache_dir(sector_id).join(".pc2_running")
}
