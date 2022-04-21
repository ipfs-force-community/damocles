use super::{Event, State, Task};
use crate::sealing::failure::*;
use anyhow::{anyhow, Result};

pub const PLANNER_NAME_SEALER: &str = "sealer";
pub const PLANNER_NAME_SNAPUP: &str = "snapup";

mod sealer;

mod snapup;

mod common;

type ExecResult = Result<Event, Failure>;

macro_rules! plan {
    ($e:expr, $st:expr, $($prev:pat => {$($evt:pat => $next:expr,)+},)*) => {
        match $st {
            $(
                $prev => {
                    match $e {
                        $(
                            $evt => $next,
                        )+
                        _ => return Err(anyhow::anyhow!("unexpected event {:?} for state {:?}", $e, $st)),
                    }
                }
            )*

            other => return Err(anyhow::anyhow!("unexpected state {:?}", other)),
        }
    };
}

pub fn get_planner(p: Option<&str>) -> Result<Box<dyn Planner>> {
    match p {
        None | Some(PLANNER_NAME_SEALER) => Ok(Box::new(sealer::SealerPlanner)),

        Some(PLANNER_NAME_SNAPUP) => Ok(Box::new(snapup::SnapUpPlanner)),

        Some(other) => Err(anyhow!("unknown planner {}", other)),
    }
}

pub(self) use plan;

pub trait Planner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State>;
    fn exec<'t>(&self, task: &'t mut Task<'_>) -> Result<Option<Event>, Failure>;
}

impl Planner for Box<dyn Planner> {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        self.as_ref().plan(evt, st)
    }

    fn exec<'t>(&self, task: &'t mut Task<'_>) -> Result<Option<Event>, Failure> {
        self.as_ref().exec(task)
    }
}
