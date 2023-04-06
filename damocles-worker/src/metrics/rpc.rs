use super::util::make_metric;

make_metric! {
    (call: counter, "rpc.call", method),
    (error: counter, "rpc.error", method),
    (timing: histogram, "rpc.timing", method),
}
