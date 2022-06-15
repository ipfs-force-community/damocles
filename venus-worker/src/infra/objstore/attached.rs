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

    /// get all available store instance names
    pub fn available_instances(&self) -> Vec<String> {
        self.stores
            .values()
            .filter_map(|st| if st.readonly() { None } else { Some(st.instance()) })
            .collect()
    }
}
