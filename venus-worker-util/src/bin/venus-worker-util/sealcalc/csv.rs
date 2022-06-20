use std::io;

use anyhow::Result;
use venus_worker_util::sealcalc;

/// Display the status of tasks running at different times by csv format
pub fn display(
    items: &[sealcalc::Item],
    pc1_concurrent: usize,
    pc2_concurrent: usize,
    c2_concurrent: usize,
    sealing_threads: usize,
) -> Result<()> {
    let mut writer = csv::Writer::from_writer(io::stdout());
    writer.write_record(&[
        "time (mins)",
        "sealing threads (free/total)",
        "pc1 (free/total)",
        "pc2 (free/total)",
        "wait seed",
        "c2 (free/total)",
        "finished sectors",
    ])?;

    for item in items {
        writer.write_record([
            item.time_in_mins.to_string(),
            format!("{}/{}", item.sealing_threads_running, sealing_threads),
            format!("{}/{}", item.pc1_running, pc1_concurrent),
            format!("{}/{}", item.pc2_running, pc2_concurrent),
            item.seed_waiting.to_string(),
            // item.seed_got.to_string(),
            format!("{}/{}", item.c2_running, c2_concurrent),
            item.finished_sectors.to_string(),
        ])?;
    }

    Ok(())
}
