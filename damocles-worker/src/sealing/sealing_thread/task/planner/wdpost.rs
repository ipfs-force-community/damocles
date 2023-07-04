use super::super::{call_rpc, Event, State, Task};
use super::{plan, ExecResult, Planner};
use crate::logging::{error, warn};
use crate::rpc::sealer::{AllocateSectorSpec, SectorID, WdPoStResult, WdpostState};
use crate::sealing::failure::MapErrToFailure;
use crate::sealing::failure::{Failure, IntoFailure};
use anyhow::{anyhow, Context, Result};
use std::time::Duration;
use tracing::debug;
use vc_processors::builtin::tasks::{PoStReplicaInfo, WindowPoSt};

pub struct WdPostPlanner;

impl Planner for WdPostPlanner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                // alloc wdpost task
                Event::AcquireWdPostTask(_) => State::Allocated,
            },
            State::Allocated => {
                // gen prove and report persistent
                Event::WdPostGenerated(_) => State::WdPostGenerated,
            },
            State::WdPostGenerated => {
                // verify prove
                Event::Finish => State::Finished,
            },
        };

        Ok(next)
    }

    fn exec(&self, task: &mut Task<'_>) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let inner = WdPost { task };

        match state {
            State::Empty => inner.acquire(),
            State::Allocated => inner.generate(),
            State::WdPostGenerated => inner.upload(),
            other => Err(anyhow!("unexpected state: {:?} in window post planner", other).abort()),
        }
        .map(From::from)
    }
}

struct WdPost<'c, 't> {
    task: &'t mut Task<'c>,
}

impl WdPost<'_, '_> {
    fn acquire(&self) -> ExecResult {
        let maybe_res = call_rpc!(
            self.task.ctx.global.rpc,
            allocate_wd_post_task,
            AllocateSectorSpec {
                allowed_miners: Some(self.task.sealing_config.allowed_miners.clone()),
                allowed_proof_types: Some(self.task.sealing_config.allowed_proof_types.clone()),
            },
        );

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!(
                    "window PoST task is not allocated yet, so we can retry even though we got the err {:?}",
                    e
                );
                return Ok(Event::Idle);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
        };

        Ok(Event::AcquireWdPostTask(allocated))
    }

    fn upload(&self) -> ExecResult {
        let out = self
            .task
            .sector
            .phases
            .wd_post_out
            .clone()
            .context("wdpost out info not found")
            .abort()?;

        let wdpost_res = WdPoStResult {
            state: WdpostState::Done,
            proofs: Some(out.proofs),
            faults: Some(out.faults),
            error: None,
        };

        self.report(wdpost_res);

        Ok(Event::Finish)
    }

    fn generate(&self) -> ExecResult {
        let task_info = self
            .task
            .sector
            .phases
            .wd_post_in
            .as_ref()
            .context("wdpost info not found")
            .abort()?;

        let instance_name = &task_info.instance;
        debug!("find access store named {}", instance_name);
        let instance = self
            .task
            .ctx
            .global
            .attached
            .get(instance_name)
            .with_context(|| format!("get access store instance named {}", instance_name))
            .perm()?;

        // get sealed path and cache path
        let replica = task_info
            .sectors
            .iter()
            .map(|sector| {
                let sector_id = &SectorID {
                    miner: task_info.miner_id,
                    number: sector.sector_id.into(),
                };

                let sealed_temp = self.task.sealed_file(sector_id);
                let sealed_rel = sealed_temp.rel();

                let cache_temp = self.task.cache_dir(sector_id);
                let cache_rel = cache_temp.rel();

                let sealed_path = instance
                    .uri(sealed_rel)
                    .with_context(|| format!("get uri for sealed file {:?} in {}", sealed_rel, instance_name))?;
                let cache_path = instance
                    .uri(cache_rel)
                    .with_context(|| format!("get uri for cache file {:?} in {}", cache_rel, instance_name))?;

                let sector_id = sector.sector_id;
                let replica = PoStReplicaInfo {
                    sector_id,
                    comm_r: sector.comm_r,
                    cache_dir: cache_path,
                    sealed_file: sealed_path,
                };
                Ok(replica)
            })
            .collect::<Result<Vec<_>>>()
            .perm()?;

        let post_in = WindowPoSt {
            miner_id: task_info.miner_id,
            proof_type: task_info.proof_type,
            replicas: replica,
            seed: task_info.seed,
        };

        let rt = tokio::runtime::Runtime::new().unwrap();
        let (tx_res, mut rx_res) = tokio::sync::oneshot::channel::<Result<()>>();
        let (tx_sync, rx_sync) = tokio::sync::oneshot::channel();

        let rpc = self.task.ctx.global.rpc.clone();
        let miner_id = task_info.miner_id;
        let deadline_id = task_info.deadline_id;

        rt.spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(20));

            let mut rep = WdPoStResult {
                state: WdpostState::Generating,
                proofs: None,
                faults: None,
                error: None,
            };

            let report = |rep: WdPoStResult| {
                if let Err(e) = call_rpc!(rpc, wd_post_heartbeat, miner_id, deadline_id, rep,) {
                    error!("report wdpost result failed: {:?}", e);
                }
            };

            loop {
                tokio::select! {
                    res = &mut rx_res => {
                        match res {
                            Ok(Ok(_)) => {
                                rep.state = WdpostState::Generated;
                                report(rep)
                            }
                            Ok(Err(e)) => {
                                rep.state = WdpostState::Failed;
                                rep.error = Some(format!("{:?}", e));
                                report(rep)
                            }
                            Err(_) => {
                                error!("receive finish signal failed");
                            }
                        }
                        break;
                    }
                    _ = interval.tick() => {
                        report(rep.clone());
                    }
                }
            }
            tx_sync.send(()).unwrap();
        });

        let _rt_guard = rt.enter();

        let out_maybe = self
            .task
            .ctx
            .global
            .processors
            .wdpost
            .process(post_in)
            .context("generate window post");

        // notify crond
        match &out_maybe {
            Ok(_) => {
                if tx_res.send(Ok(())).is_err() {
                    warn!("send finish signal failed");
                }
            }
            Err(e) => {
                if tx_res.send(Err(anyhow!("generate window post failed: {:?}", e))).is_err() {
                    warn!("send finish signal failed");
                }
                warn!("generate window post failed: {:?}", e);
            }
        };

        // wait for crond to finish
        rx_sync.blocking_recv().unwrap();

        let out = out_maybe.context("generate window post").temp()?;

        Ok(Event::WdPostGenerated(out))
    }

    fn report(&self, res: WdPoStResult) {
        if let Some(task_info) = self.task.sector.phases.wd_post_in.as_ref() {
            let resp = call_rpc!(
                self.task.ctx.global.rpc,
                wd_post_heartbeat,
                task_info.miner_id,
                task_info.deadline_id,
                res,
            );
            if let Err(e) = resp {
                warn!("report wdpost result failed: {:?}", e);
            }
        } else {
            warn!("wdpost info not found");
        }
    }
}
