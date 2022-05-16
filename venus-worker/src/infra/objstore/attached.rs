//! manages multiple attached stores

use std::collections::HashMap;

use anyhow::{anyhow, Result};
use rand::{rngs::OsRng, seq::SliceRandom};

use super::ObjectStore;
use crate::logging::{debug, warn};

/// manages all attached stores
pub struct AttachedManager {
    enable_space_weighted: bool,
    stores: HashMap<String, Box<dyn ObjectStore>>,
}

impl AttachedManager {
    /// init AttachedManager with given stores
    pub fn init(enable_space_weighted: bool, attached: Vec<Box<dyn ObjectStore>>) -> Result<Self> {
        let mut stores = HashMap::new();
        for astore in attached {
            if let Some(prev) = stores.insert(astore.instance(), astore) {
                return Err(anyhow!("duplicate instance name {}", prev.instance()));
            };
        }

        Ok(AttachedManager {
            enable_space_weighted,
            stores,
        })
    }

    /// get a named store instance
    pub fn get(&self, instance: &str) -> Option<&dyn ObjectStore> {
        self.stores.get(instance).map(|b| b.as_ref())
    }

    /// acquire an available store for sector persistence
    pub fn acquire_persist(&self, size: u64, prev_instance: Option<String>) -> Option<&dyn ObjectStore> {
        let picker = |s: &dyn ObjectStore| -> Option<u64> {
            if s.readonly() {
                return None;
            }

            let free = match s.free_space() {
                Ok(space) => space,
                Err(e) => {
                    warn!("get free space: {:?}", e);
                    0
                }
            };

            if free <= size {
                return None;
            }

            Some(free)
        };

        if let Some(ins) = prev_instance
            .as_ref()
            .and_then(|name| self.stores.get(name))
            .and_then(|s| picker(s.as_ref()).map(|_| s))
        {
            return Some(ins.as_ref());
        };

        let weighted_instances = self
            .stores
            .values()
            .filter_map(|s| picker(s.as_ref()).map(|free| (s, free)))
            .collect::<Vec<_>>();

        let selected = if self.enable_space_weighted {
            weighted_instances.choose_weighted(&mut OsRng, |ins| ins.1).ok()
        } else {
            weighted_instances.choose(&mut OsRng)
        };

        selected.map(|ins| {
            let space = byte_unit::Byte::from(ins.1).get_appropriate_unit(true);
            debug!(free = %space.to_string(), instance = %ins.0.instance(), "store selected");
            ins.0.as_ref()
        })
    }
}
