//! manages multiple attached stores

use std::collections::HashMap;

use anyhow::{anyhow, Result};

use super::ObjectStore;

/// manages all attached stores
pub struct AttachedManager {
    stores: HashMap<String, Box<dyn ObjectStore>>,
}

impl AttachedManager {
    /// init AttachedManager with given stores
    pub fn init(attached: Vec<Box<dyn ObjectStore>>) -> Result<Self> {
        let mut stores = HashMap::new();
        for astore in attached {
            if let Some(prev) = stores.insert(astore.instance(), astore) {
                return Err(anyhow!("duplicate instance name {}", prev.instance()));
            };
        }

        Ok(AttachedManager { stores })
    }

    /// get a named store instance
    pub fn get(&self, instance: &str) -> Option<&dyn ObjectStore> {
        self.stores.get(instance).map(|b| b.as_ref())
    }

    /// acquire an available store for sector persistence
    pub fn acquire_persist(
        &self,
        _size: u64,
        prev_instance: Option<String>,
    ) -> Option<&dyn ObjectStore> {
        if let Some(ins) = prev_instance
            .as_ref()
            .and_then(|name| self.stores.get(name))
        {
            if !ins.readonly() {
                return Some(ins.as_ref());
            }
        };

        // TODO: depends on the free space
        self.stores
            .values()
            .find(|s| !s.readonly())
            .map(|ins| ins.as_ref())
    }
}
