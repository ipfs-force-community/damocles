use crate::{sealing::failure::*, watchdog::Ctx};
use anyhow::{anyhow, Result};

pub const PLANNER_NAME_SEALER: &str = "sealer";
pub const PLANNER_NAME_SNAPUP: &str = "snapup";
pub const PLANNER_NAME_REBUILD: &str = "rebuild";
pub const PLANNER_NAME_UNSEAL: &str = "unseal";
pub const PLANNER_NAME_WDPOST: &str = "wdpost";
pub const PLANNER_NAME_NIPOREP: &str = "niporep";

mod common;
mod niporep;
mod rebuild;
mod sealer;
mod snapup;
mod unseal;
mod wdpost;

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

pub fn default_plan() -> &'static str {
    PLANNER_NAME_SEALER
}

use niporep::NiPoRepPlanner;
use plan;

use self::{
    common::CommonSealer, rebuild::RebuildPlanner, sealer::SealerPlanner,
    snapup::SnapUpPlanner, unseal::UnsealPlanner, wdpost::WdPostSealer,
};

use super::{Sealer, SealingThread};

pub trait JobTrait {
    fn planner(&self) -> &str;
}

pub trait PlannerTrait: Default {
    type Job: JobTrait;
    type State;
    type Event;

    fn name(&self) -> &str;
    fn plan(&self, evt: &Self::Event, st: &Self::State) -> Result<Self::State>;
    fn exec(&self, job: &mut Self::Job)
        -> Result<Option<Self::Event>, Failure>;
    fn apply(
        &self,
        event: Self::Event,
        state: Self::State,
        job: &mut Self::Job,
    ) -> Result<()>;
}

pub(crate) fn create_sealer(
    plan: &str,
    ctx: &Ctx,
    st: &SealingThread,
) -> Result<Box<dyn Sealer>> {
    match plan {
        PLANNER_NAME_SEALER => {
            Ok(Box::new(CommonSealer::<SealerPlanner>::new(ctx, st)?))
        }
        PLANNER_NAME_SNAPUP => {
            Ok(Box::new(CommonSealer::<SnapUpPlanner>::new(ctx, st)?))
        }
        PLANNER_NAME_REBUILD => {
            Ok(Box::new(CommonSealer::<RebuildPlanner>::new(ctx, st)?))
        }
        PLANNER_NAME_UNSEAL => {
            Ok(Box::new(CommonSealer::<UnsealPlanner>::new(ctx, st)?))
        }
        PLANNER_NAME_WDPOST => {
            Ok(Box::new(WdPostSealer::new(st.sealing_ctrl(ctx))))
        }
        PLANNER_NAME_NIPOREP => {
            Ok(Box::new(CommonSealer::<NiPoRepPlanner>::new(ctx, st)?))
        }
        unknown => Err(anyhow!("unknown planner: {}", unknown)),
    }
}
