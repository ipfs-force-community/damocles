use std::{backtrace::Backtrace, panic::PanicInfo};

/// Exit the whole process when panic.
pub fn install_panic_hook(panic_abort: bool) {
    let old_hook = std::panic::take_hook();
    // Set a panic hook that records the panic as a `tracing` event at the
    // `ERROR` verbosity level.
    //
    // If we are currently in a span when the panic occurred, the logged event
    // will include the current span, allowing the context in which the panic
    // occurred to be recorded.
    std::panic::set_hook(Box::new(move |panic| {
        log_panic(panic);
        old_hook(panic);
        if panic_abort {
            std::process::abort();
        } else {
            unsafe {
                // Calling process::exit would trigger global static to destroy,
                // So calling libc::_exit() instead to skip the cleanup process.
                libc::_exit(1);
            }
        }
    }));
}

fn log_panic(panic: &PanicInfo) {
    let backtrace = Backtrace::force_capture();
    let backtrace_str = format!("{:?}", backtrace);

    eprintln!("{}", panic);
    eprintln!("{}", backtrace);

    if let Some(location) = panic.location() {
        tracing::error!(
            message = %panic,
            backtrace = %backtrace_str,
            panic.file = location.file(),
            panic.line = location.line(),
            panic.column = location.column(),
        );
    } else {
        tracing::error!(message = %panic, backtrace = %backtrace_str);
    }
}
