use anyhow::Result;

use crate::builtin::tasks::{SupraC1, SupraPC1, SupraPC2};
use crate::core::{Processor, Task};

use super::BuiltinProcessor;

impl Processor<SupraPC1> for BuiltinProcessor {
    fn name(&self) -> String {
        "builtin SupraPC1".to_string()
    }

    fn process(&self, _task: SupraPC1) -> Result<<SupraPC1 as Task>::Output> {
        unimplemented!()
    }
}

impl Processor<SupraPC2> for BuiltinProcessor {
    fn name(&self) -> String {
        "builtin SupraPC2".to_string()
    }

    fn process(&self, _task: SupraPC2) -> Result<<SupraPC2 as Task>::Output> {
        unimplemented!()
    }
}

impl Processor<SupraC1> for BuiltinProcessor {
    fn name(&self) -> String {
        "builtin SupraC1".to_string()
    }

    fn process(&self, _task: SupraC1) -> Result<<SupraC1 as Task>::Output> {
        unimplemented!()
    }
}
