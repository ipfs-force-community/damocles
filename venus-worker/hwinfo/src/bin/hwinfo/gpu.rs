use anyhow::Result;
use clap::{App, ArgMatches, SubCommand};
use hwinfo::{byte_string, gpu};
use term_table::{
    row::Row,
    table_cell::{Alignment, TableCell},
    Table, TableStyle,
};

pub const SUB_CMD_NAME: &str = "gpu";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    const ABOUT: &str = "print gpu infomation";
    SubCommand::with_name(SUB_CMD_NAME).about(ABOUT).help_message(ABOUT)
}

pub(crate) fn submatch(_subargs: &ArgMatches<'_>) -> Result<()> {
    render_gpu();
    Ok(())
}

fn render_gpu() {
    let mut table = Table::new();
    table.style = TableStyle::rounded();
    table.add_row(Row::new(vec![
        TableCell::new_with_alignment("name", 1, Alignment::Center),
        TableCell::new_with_alignment("vendor", 1, Alignment::Center),
        TableCell::new_with_alignment("memory", 1, Alignment::Center),
    ]));
    let gpus = gpu::load();
    if gpus.is_empty() {
        table.add_row(Row::new(vec![TableCell::new_with_alignment(
            "No GPU device detected",
            3,
            Alignment::Center,
        )]));
    } else {
        for gpu_info in gpus {
            table.add_row(Row::new(vec![
                TableCell::new(gpu_info.name),
                TableCell::new(gpu_info.vendor.as_ref()),
                TableCell::new(format!("{:.2}%", byte_string(gpu_info.memory, 2))),
            ]));
        }
    }

    println!("{}", table.render());
}
