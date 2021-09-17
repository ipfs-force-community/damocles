use anyhow::Result;
use signal_hook::{
    consts::{SIGINT, SIGQUIT, SIGTERM, TERM_SIGNALS},
    iterator::Signals,
};

use crate::logging::warn;
use crate::watchdog::{Ctx, Module};

pub struct Signal;

impl Module for Signal {
    fn should_wait(&self) -> bool {
        true
    }

    fn id(&self) -> String {
        "signal".to_owned()
    }

    fn run(&mut self, _ctx: Ctx) -> Result<()> {
        let mut sig = Signals::new(TERM_SIGNALS)?;
        for signal in sig.forever() {
            match signal {
                SIGINT | SIGQUIT | SIGTERM => {
                    warn!("captured signal {}", signal);
                    break;
                }

                _ => {}
            }
        }

        Ok(())
    }
}
