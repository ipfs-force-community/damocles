pub mod tower;

use crate::{Processor, Task};

use self::tower::TowerWrapper;

impl<P: ?Sized, T: Task> ProcessorExt<T> for P where P: Processor<T> {}

/// An extension trait for `Processor`s that provides a variety of convenient
/// adapters
pub trait ProcessorExt<Tsk: Task>: Processor<Tsk> {
    fn tower(self) -> TowerWrapper<Self>
    where
        Self: Sized,
    {
        TowerWrapper(self)
    }
}
