use super::util::make_metric;

make_metric! {
    (call: counter, "rpc.call", method),
    (timing: counter, "rpc.timing", method),
}
