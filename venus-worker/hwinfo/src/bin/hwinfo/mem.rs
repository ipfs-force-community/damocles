use anyhow::Result;
use clap::{App, ArgMatches, SubCommand};
use hwinfo::{byte_string, mem};
use term_table::{
    row::Row,
    table_cell::{Alignment, TableCell},
    Table, TableStyle,
};

pub const SUB_CMD_NAME: &str = "mem";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    const ABOUT: &str = "print memory infomation";
    SubCommand::with_name(SUB_CMD_NAME).about(ABOUT).help_message(ABOUT)
}

pub(crate) fn submatch(_subargs: &ArgMatches<'_>) -> Result<()> {
    render_mem();
    Ok(())
}

fn render_mem() {
    let mem_info = mem::load();
    let mut table = Table::new();
    table.style = TableStyle::rounded();
    table.add_row(Row::new(vec![
        TableCell::new_with_alignment("Total memory", 1, Alignment::Center),
        TableCell::new_with_alignment("Used memory", 1, Alignment::Center),
        TableCell::new_with_alignment("Total swap", 1, Alignment::Center),
        TableCell::new_with_alignment("Used swap", 1, Alignment::Center),
    ]));

    table.add_row(Row::new(vec![
        TableCell::new(byte_string(mem_info.total_mem, 2)),
        TableCell::new(format!(
            "{} ({:.2}%)",
            byte_string(mem_info.used_mem, 2),
            (mem_info.used_mem as f32 / mem_info.total_mem as f32) * 100.0
        )),
        TableCell::new(byte_string(mem_info.total_swap, 2)),
        TableCell::new(format!(
            "{} ({:.2}%)",
            byte_string(mem_info.used_swap, 2),
            (mem_info.used_swap as f32 / mem_info.total_swap as f32) * 100.0
        )),
    ]));
    println!("{}", table.render());
}
