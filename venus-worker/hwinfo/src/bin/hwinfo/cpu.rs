use anyhow::{anyhow, Result};
use clap::{App, ArgMatches, SubCommand};
use hwinfo::cpu;

pub const SUB_CMD_NAME: &str = "cpu";

pub(crate) fn subcommand<'a, 'b>() -> App<'a, 'b> {
    const ABOUT: &str = "print CPU infomation";
    SubCommand::with_name(SUB_CMD_NAME).about(ABOUT).help_message(ABOUT)
}

pub(crate) fn submatch(_subargs: &ArgMatches<'_>) -> Result<()> {
    render_cpu()
}

fn render_cpu() -> Result<()> {
    fn walk(parent: &cpu::TopologyNode, prefix: &str) {
        let mut index = parent.children.len();

        for child_topo_node in &parent.children {
            let info = child_topo_node.to_string();
            index -= 1;

            if index == 0 {
                println!("{}└── {}", prefix, info);
                walk(child_topo_node, &format!("{}    ", prefix));
            } else {
                println!("{}├── {}", prefix, info);
                walk(child_topo_node, &format!("{}│   ", prefix));
            }
        }
    }

    let machine = cpu::load().ok_or_else(|| anyhow!("Can not load cpu infomation"))?;
    println!("{}", machine.to_string());
    walk(&machine, "");
    Ok(())
}
