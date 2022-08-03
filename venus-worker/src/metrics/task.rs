use super::util::make_metric;

make_metric! {
    (threads_pause: gauge, "task.threads.pause"),
}
