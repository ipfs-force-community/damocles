use std::collections::HashMap;

/// The status of each task running at a specific time period
pub struct Item {
    /// Time in minutes
    pub time_in_mins: usize,
    /// Number of sealing threads running in the current minute
    pub sealing_threads_running: usize,
    /// Number of tree_d tasks running in the current minute
    pub tree_d_running: usize,
    /// Number of pc1 tasks running in the current minute
    pub pc1_running: usize,
    /// Number of pc2 tasks running in the current minute
    pub pc2_running: usize,
    /// Number of tasks waiting for seed in the current minute
    pub seed_waiting: usize,
    /// Number of c2 tasks running in the current minute
    pub c2_running: usize,
    /// Number of finished sectors in the current minute
    pub finished_sectors: usize,
}

struct TaskStatus {
    required_mins: usize,
    max_concurrent: usize,
    running_num: usize,
    done: usize,
    // running tasks <start_min, number of tasks>
    running: HashMap<usize, usize>,
}

impl TaskStatus {
    fn new(required_mins: usize, max_concurrent: usize) -> Self {
        Self {
            required_mins,
            max_concurrent,
            running_num: 0,
            done: 0,
            running: HashMap::new(),
        }
    }

    fn step(&mut self, current_min: usize, pending_tasks: &mut usize) -> usize {
        let mut current_step_finished = 0;
        // finish tasks
        self.running.retain(|start_min, tasks| {
            if current_min - start_min == self.required_mins {
                // finished
                current_step_finished += *tasks;
                return false;
            }
            // not finish
            true
        });
        self.running_num -= current_step_finished;
        self.done += current_step_finished;

        // start tasks
        let can_run = self.free().min(*pending_tasks);
        if can_run > 0 {
            self.running.insert(current_min, can_run);
            self.running_num += can_run;
            *pending_tasks -= can_run;
        }

        current_step_finished
    }

    fn done_mut(&mut self) -> &mut usize {
        &mut self.done
    }

    fn free(&self) -> usize {
        self.max_concurrent - self.running_num
    }
}

/// Calculate the running state of each task at different time periods
/// by the given running minutes and maximum number concurrent of each task
/// to guide the configuration of venus-worker parameters
pub fn calc(
    (tree_d_mins, tree_d_concurrent): (usize, usize),
    (pc1_mins, pc1_concurrent): (usize, usize),
    (pc2_mins, pc2_concurrent): (usize, usize),
    (c2_mins, c2_concurrent): (usize, usize),
    seed_mins: usize,
    sealing_threads: usize,
    (calculate_mins, output_step): (usize, usize),
) -> Vec<Item> {
    let mut sealing_threads_free = sealing_threads;

    let mut tree_d = TaskStatus::new(tree_d_mins, tree_d_concurrent);
    let mut pc1 = TaskStatus::new(pc1_mins, pc1_concurrent);
    let mut pc2 = TaskStatus::new(pc2_mins, pc2_concurrent);
    let mut wait_seed = TaskStatus::new(seed_mins, usize::max_value());
    let mut c2 = TaskStatus::new(c2_mins, c2_concurrent);

    let mut all_data = Vec::with_capacity(calculate_mins);

    for m in 0..calculate_mins {
        tree_d.step(m, &mut sealing_threads_free);
        pc1.step(m, tree_d.done_mut());
        pc2.step(m, pc1.done_mut());
        wait_seed.step(m, pc2.done_mut());
        sealing_threads_free += c2.step(m, wait_seed.done_mut());

        if m % output_step == 0 {
            all_data.push(Item {
                time_in_mins: m,
                sealing_threads_running: sealing_threads - sealing_threads_free,
                tree_d_running: tree_d.running_num,
                pc1_running: pc1.running_num,
                pc2_running: pc2.running_num,
                seed_waiting: wait_seed.running_num,
                c2_running: c2.running_num,
                finished_sectors: c2.done,
            });
        }
    }

    all_data
}

#[cfg(test)]
mod tests {
    use super::TaskStatus;

    #[test]
    fn test_task_status_step() {
        struct TestCase {
            current_min: usize,
            pending_tasks: usize,
            expected_pending_tasks: usize,
            expected_running_num: usize,
            expected_done: usize,
        }
        let cases = vec![
            (
                TaskStatus::new(1, 2),
                vec![
                    TestCase {
                        current_min: 0,
                        pending_tasks: 5,
                        expected_pending_tasks: 3,
                        expected_running_num: 2,
                        expected_done: 0,
                    },
                    TestCase {
                        current_min: 1,
                        pending_tasks: 5,
                        expected_pending_tasks: 3,
                        expected_running_num: 2,
                        expected_done: 2,
                    },
                    TestCase {
                        current_min: 2,
                        pending_tasks: 1,
                        expected_pending_tasks: 0,
                        expected_running_num: 1,
                        expected_done: 4,
                    },
                ],
            ),
            (
                TaskStatus::new(2, 2),
                vec![
                    TestCase {
                        current_min: 0,
                        pending_tasks: 5,
                        expected_pending_tasks: 3,
                        expected_running_num: 2,
                        expected_done: 0,
                    },
                    TestCase {
                        current_min: 1,
                        pending_tasks: 5,
                        expected_pending_tasks: 5,
                        expected_running_num: 2,
                        expected_done: 0,
                    },
                ],
            ),
        ];

        for (mut task_status, test_cases) in cases {
            for mut test_case in test_cases {
                task_status.step(test_case.current_min, &mut test_case.pending_tasks);
                assert_eq!(
                    test_case.pending_tasks, test_case.expected_pending_tasks,
                    "expected pending_tasks: {}, got: {}",
                    test_case.expected_pending_tasks, test_case.pending_tasks
                );
                assert_eq!(
                    task_status.running_num, test_case.expected_running_num,
                    "expected running_num: {}, got: {}",
                    test_case.expected_running_num, task_status.running_num
                );
                assert_eq!(
                    task_status.done, test_case.expected_done,
                    "expected done: {}, got: {}",
                    test_case.expected_done, task_status.done
                );
            }
        }
    }
}
