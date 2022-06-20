use std::collections::HashMap;

const SEED_WAITING_IN_MINS: usize = 80;

/// The status of each task running at a specific time period
pub struct Item {
    /// Time in minutes
    pub time_in_mins: usize,
    /// Number of sealing threads running in the current minute
    pub sealing_threads_running: usize,
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

/// Calculate the running state of each task at different time periods
/// by using the given running minutes of each task and the maximum number
/// of concurrent to guide the configuration of venus-worker parameters
pub fn calc(
    (pc1_mins, pc1_concurrent): (usize, usize),
    (pc2_mins, pc2_concurrent): (usize, usize),
    (c2_mins, c2_concurrent): (usize, usize),
    sealing_threads: usize,
    calculate_mins: usize,
) -> Vec<Item> {
    let mut sectors = 0;

    let mut sealing_threads_free = sealing_threads;
    let mut pc1_free = pc1_concurrent;
    let mut pc1_done = 0;
    let mut pc1_running = HashMap::new();

    let mut pc2_free = pc2_concurrent;
    let mut pc2_done = 0;
    let mut pc2_running = HashMap::new();

    let mut c2_free = c2_concurrent;
    let mut c2_running = HashMap::new();

    let mut seed_waiting = HashMap::new();
    let mut seed_wait = 0;
    let mut seed_done = 0;

    let mut all_data = Vec::with_capacity(calculate_mins);
    for m in 0..calculate_mins {
        // finish p1
        pc1_running.retain(|start, tasks| {
            if m - start != pc1_mins {
                // not finish
                return true;
            }
            // finished
            pc1_free += *tasks;
            pc1_done += *tasks;
            false
        });

        // start p1
        if pc1_free > 0 && sealing_threads_free > 0 {
            let can_run = pc1_free.min(sealing_threads_free);
            pc1_running.insert(m, can_run);
            pc1_free -= can_run;
            sealing_threads_free -= can_run;
        }

        // finish p2
        pc2_running.retain(|start, tasks| {
            if m - start != pc2_mins {
                // not finish
                return true;
            }
            // finished
            pc2_free += *tasks;
            pc2_done += *tasks;
            false
        });

        // start p2
        if pc1_done > 0 && pc2_free > 0 {
            let can_run = pc1_done.min(pc2_free);
            pc2_running.insert(m, can_run);
            pc1_done -= can_run;
            pc2_free -= can_run;
        }

        // finish seed wait
        seed_waiting.retain(|start, tasks| {
            if m - start != SEED_WAITING_IN_MINS {
                // not finish
                return true;
            }
            // finished
            seed_wait -= *tasks;
            seed_done += *tasks;
            false
        });

        // start wait seed
        if pc2_done > 0 {
            seed_waiting.insert(m, pc2_done);
            seed_wait += pc2_done;
            pc2_done = 0;
        }

        c2_running.retain(|start, tasks| {
            if m - start != c2_mins {
                // not finish
                return true;
            }
            // finished
            c2_free += *tasks;
            sealing_threads_free += *tasks;
            sectors += *tasks;
            false
        });

        // start c2
        if seed_done > 0 && c2_free > 0 {
            let can_run = seed_done.min(c2_free);
            c2_running.insert(m, can_run);
            seed_done -= can_run;
            c2_free -= can_run;
        }

        if m % 60 == 0 {
            all_data.push(Item {
                time_in_mins: m,
                sealing_threads_running: sealing_threads - sealing_threads_free,
                pc1_running: pc1_concurrent - pc1_free,
                pc2_running: pc2_concurrent - pc2_free,
                seed_waiting: seed_wait,
                c2_running: c2_concurrent - c2_free,
                finished_sectors: sectors,
            });
        }
    }

    all_data
}
