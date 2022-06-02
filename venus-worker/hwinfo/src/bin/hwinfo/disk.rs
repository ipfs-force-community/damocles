use anyhow::Result;
use clap::{App, ArgMatches, SubCommand};
use hwinfo::{byte_string, disk};
use term_table::{
    row::Row,
    table_cell::{Alignment, TableCell},
    Table, TableStyle,
};

pub const SUB_CMD_NAME: &str = "disk";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    const ABOUT: &str = "print disks infomation";
    SubCommand::with_name(SUB_CMD_NAME).about(ABOUT).help_message(ABOUT)
}

pub(crate) fn submatch(_subargs: &ArgMatches<'_>) -> Result<()> {
    render_disk();
    Ok(())
}

fn render_disk() {
    let disks = disk::load();
    let mut table = Table::new();
    table.style = TableStyle::rounded();
    table.add_row(Row::new(vec![
        TableCell::new_with_alignment("Disk type", 1, Alignment::Center),
        TableCell::new_with_alignment("Device name", 1, Alignment::Center),
        TableCell::new_with_alignment("Filesystem", 1, Alignment::Center),
        TableCell::new_with_alignment("Space", 1, Alignment::Center),
    ]));

    for disk in &disks {
        let used_bytes = disk.total_space - disk.available_space;
        table.add_row(Row::new(vec![
            TableCell::new(disk.disk_type.as_ref()),
            TableCell::new(&disk.device_name),
            TableCell::new(&disk.filesystem),
            TableCell::new(format!(
                "{} / {} ({:.2}% used)",
                byte_string(used_bytes, 2),
                byte_string(disk.total_space, 2),
                (used_bytes as f32 / disk.total_space as f32 * 100.0)
            )),
        ]))
    }
    println!("{}", table.render());
}
