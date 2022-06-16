use anyhow::{Context, Result};
use clap::{Arg, ArgAction, Command};
use hwinfo::{byte_string, cpu, disk, gpu, mem};
use itertools::Itertools;
use term_table::{
    row::Row,
    table_cell::{Alignment, TableCell},
    Table, TableStyle,
};

fn main() -> Result<()> {
    let ver_string = format!(
        "v{}-{}",
        env!("CARGO_PKG_VERSION"),
        option_env!("GIT_COMMIT").unwrap_or("dev")
    );

    let matches = Command::new("hwinfo")
        .version(ver_string.as_str())
        .about("Show hardware information")
        .arg(
            Arg::new("full")
                .long("full")
                .help("Show full CPU topology")
                .action(ArgAction::SetTrue),
        )
        .get_matches();

    println!("CPU topology:");
    render_cpu(*matches.get_one::<bool>("full").unwrap_or(&false))?;
    println!();
    println!("Disks:");
    render_disk();
    println!();
    println!("GPU:");
    render_gpu();
    println!();
    println!("Memory:");
    render_mem();
    Ok(())
}

fn render_cpu(full: bool) -> Result<()> {
    fn find_subnode(
        parent: &cpu::TopologyNode,
        filter_fn: fn(&cpu::TopologyNode) -> bool,
    ) -> Vec<&cpu::TopologyNode> {
        let mut s = Vec::new();
        for child_topo_node in &parent.children {
            if filter_fn(child_topo_node) {
                s.push(child_topo_node)
            }
            s.extend(find_subnode(child_topo_node, filter_fn));
        }
        s
    }

    fn short(nodes: Vec<&cpu::TopologyNode>) -> String {
        match nodes.as_slice() {
            n if nodes.len() <= 8 => n.iter().join(" + "),
            [f1, f2, f3, .., l3, l2, l1] => {
                format!("{} + {} + {} + ... + {} + {} + {}", f1, f2, f3, l3, l2, l1)
            }
            _ => unreachable!(),
        }
    }

    fn walk(parent: &cpu::TopologyNode, prefix: &str, full: bool) {
        if matches!(parent.ty, cpu::TopologyType::Cache { .. }) && !full {
            // output short view
            println!(
                "{}└── {}",
                prefix,
                short(find_subnode(parent, |topo| matches!(
                    topo.ty,
                    cpu::TopologyType::PU
                )))
            );
            return;
        }

        let mut index = parent.children.len();

        for child_topo_node in &parent.children {
            let info = child_topo_node.to_string();
            index -= 1;
            if index == 0 {
                println!("{}└── {}", prefix, info);
                walk(child_topo_node, &format!("{}    ", prefix), full);
            } else {
                println!("{}├── {}", prefix, info);
                walk(child_topo_node, &format!("{}│   ", prefix), full);
            }
        }
    }

    let machine = cpu::load().context("Can not load cpu information")?;
    println!("{}", machine);
    walk(&machine, "", full);
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

    if disks.is_empty() {
        table.add_row(Row::new(vec![TableCell::new_with_alignment(
            "No Disk device detected",
            4,
            Alignment::Center,
        )]));
    } else {
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
                    if disk.total_space == 0 {
                        0.0
                    } else {
                        used_bytes as f32 / disk.total_space as f32 * 100.0
                    }
                )),
            ]))
        }
    }

    println!("{}", table.render());
}

fn render_gpu() {
    let mut table = Table::new();
    table.style = TableStyle::rounded();
    table.add_row(Row::new(vec![
        TableCell::new_with_alignment("Name", 1, Alignment::Center),
        TableCell::new_with_alignment("Vendor", 1, Alignment::Center),
        TableCell::new_with_alignment("Memory", 1, Alignment::Center),
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
                TableCell::new(byte_string(gpu_info.memory, 2)),
            ]));
        }
    }

    println!("{}", table.render());
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
            if mem_info.total_swap == 0 {
                0.0
            } else {
                mem_info.used_swap as f32 / mem_info.total_swap as f32 * 100.0
            }
        )),
    ]));
    println!("{}", table.render());
}
