use std::future::Future;

use tokio::runtime::Handle;

/// shortcut for execution a future on current thread
#[inline]
pub fn block_on<F: Future>(fut: F) -> F::Output {
    Handle::current().block_on(fut)
}
