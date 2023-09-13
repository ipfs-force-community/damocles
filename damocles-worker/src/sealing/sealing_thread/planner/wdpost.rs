use std::collections::HashMap;
use std::fmt::Display;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use crossbeam_channel::{bounded, Receiver, Sender};
use jsonrpc_core::ErrorCode;
use jsonrpc_core_client::RpcError;
use tokio::runtime::Handle;
use vc_processors::builtin::tasks::{
    PoStReplicaInfo, WindowPoSt, WindowPoStOutput, STAGE_NAME_WINDOW_POST,
};
use vc_processors::fil_proofs::SectorId;

use crate::filestore::Resource;
use crate::infra::filestore::FileStoreExt;
use crate::logging::warn;
use crate::rpc::sealer::{
    AllocatePoStSpec, AllocatedWdPoStJob, PathType, SectorID, StoreResource,
    WdPoStInput,
};
use crate::sealing::failure::*;
use crate::sealing::paths;
use crate::sealing::sealing_thread::{planner::plan, Sealer, SealingCtrl, R};

use super::super::call_rpc;
use super::{JobTrait, PlannerTrait, PLANNER_NAME_WDPOST};

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
pub enum WdPostState {
    Empty,
    Allocated,
    Generated,
    Finished,
    Aborted,
}

impl WdPostState {
    pub fn from_str(s: &str) -> Option<Self> {
        Some(match s {
            "Empty" => Self::Empty,
            "Allocated" => Self::Allocated,
            "Generated" => Self::Generated,
            "Finished" => Self::Finished,
            "Aborted" => Self::Aborted,
            _ => return None,
        })
    }

    pub fn stage(&self) -> &'static str {
        match self {
            WdPostState::Empty
            | WdPostState::Finished
            | WdPostState::Aborted => "-",
            WdPostState::Allocated => "Generate",
            WdPostState::Generated => "SubmitProof",
        }
    }
}

impl Display for WdPostState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "{}",
            match self {
                WdPostState::Empty => "Empty",
                WdPostState::Allocated => "Allocated",
                WdPostState::Generated => "Generated",
                WdPostState::Finished => "Finished",
                WdPostState::Aborted => "Aborted",
            }
        )
    }
}

impl Default for WdPostState {
    fn default() -> Self {
        Self::Empty
    }
}

#[derive(Clone, Debug)]
pub enum WdPostEvent {
    Idle,
    #[allow(dead_code)]
    Retry,
    SetState(WdPostState),
    Allocated {
        allocated: AllocatedWdPoStJob,
        stop_heartbeat_tx: Sender<()>,
    },
    Generate(Result<WindowPoStOutput, String>),
    Finish,
}

pub(crate) struct WdPostSealer {
    job: WdPostJob,
    planner: WdPostPlanner,
    retry: u32,
}

impl WdPostSealer {
    pub fn new(ctrl: SealingCtrl<'static>) -> Self {
        Self {
            job: WdPostJob::new(ctrl),
            planner: WdPostPlanner,
            retry: 0,
        }
    }
}

impl Sealer for WdPostSealer {
    fn seal(&mut self, state: Option<&str>) -> Result<R, Failure> {
        let mut event = state
            .and_then(WdPostState::from_str)
            .map(WdPostEvent::SetState);
        if let (true, Some(s)) = (event.is_none(), state) {
            tracing::error!("unknown state: {}", s);
        }

        loop {
            self.job.sealing_ctrl.interrupted()?;

            if self.planner.name() != self.job.planner() {
                // switch planner
                return Ok(R::SwitchPlanner(self.job.planner().to_string()));
            }

            if let Some(evt) = event.take() {
                match &evt {
                    WdPostEvent::Idle | WdPostEvent::Retry => {
                        let recover_interval =
                            self.job.sealing_ctrl.config().recover_interval;
                        tracing::debug!(
                            sleep = ?recover_interval,
                            "Event::{:?} captured", evt
                        );

                        self.job
                            .sealing_ctrl
                            .wait_or_interrupted(recover_interval)?;
                    }

                    other => {
                        let next = if let WdPostEvent::SetState(s) = other {
                            *s
                        } else {
                            self.planner.plan(other, &self.job.state).crit()?
                        };

                        self.planner
                            .apply(evt, next, &mut self.job)
                            .context("event apply")
                            .crit()?;
                    }
                };
            };

            let span = tracing::warn_span!("handle", current = ?self.job.state);

            let _enter = span.enter();
            self.job
                .sealing_ctrl
                .ctrl_ctx()
                .update_state(|cst| {
                    let _ = cst.job.state.replace(self.job.state.to_string());
                    let _ = cst
                        .job
                        .stage
                        .replace(self.job.state.stage().to_string());
                    cst.job.id =
                        self.job.wdpost_job.as_ref().map(|t| t.id.to_owned());
                })
                .crit()?;

            tracing::debug!("handling");

            let res = self.planner.exec(&mut self.job);

            match res {
                Ok(Some(evt)) => {
                    event.replace(evt);
                }
                Ok(None) => return Ok(R::Done),
                Err(Failure(Level::Temporary, terr)) => {
                    if self.retry >= self.job.sealing_ctrl.config().max_retries
                    {
                        // reset retry times;
                        self.retry = 0;
                        return Err(terr.abort());
                    }
                    tracing::warn!(
                        retry = self.retry,
                        "temp error occurred: {:?}",
                        terr
                    );
                    self.retry += 1;
                    tracing::info!(
                        interval = ?self.job.sealing_ctrl.config().recover_interval,
                        "wait before recovering"
                    );

                    self.job.sealing_ctrl.wait_or_interrupted(
                        self.job.sealing_ctrl.config().recover_interval,
                    )?;
                }

                Err(f) => return Err(f),
            }
        }
    }
}

#[derive(Clone)]
pub struct WdPostJob {
    sealing_ctrl: SealingCtrl<'static>,

    state: WdPostState,
    wdpost_job: Option<AllocatedWdPoStJob>,
    wdpost_job_result: Option<Result<WindowPoStOutput, String>>,

    stop_heartbeat_tx: Option<Sender<()>>,
}

impl JobTrait for WdPostJob {
    fn planner(&self) -> &str {
        self.sealing_ctrl.config().plan()
    }
}

impl WdPostJob {
    fn new(sealing_ctrl: SealingCtrl<'static>) -> Self {
        WdPostJob {
            sealing_ctrl,
            state: WdPostState::default(),
            wdpost_job: None,
            wdpost_job_result: None,
            stop_heartbeat_tx: None,
        }
    }
}

#[derive(Default)]
pub struct WdPostPlanner;

impl PlannerTrait for WdPostPlanner {
    type Job = WdPostJob;
    type State = WdPostState;
    type Event = WdPostEvent;

    fn name(&self) -> &str {
        PLANNER_NAME_WDPOST
    }

    fn plan(&self, evt: &Self::Event, st: &Self::State) -> Result<Self::State> {
        let next = plan! {
            evt,
            st,

            WdPostState::Empty => {
                // alloc wdpost job
                WdPostEvent::Allocated{ .. } => WdPostState::Allocated,
            },
            WdPostState::Allocated => {
                // gen prove
                WdPostEvent::Generate(_) => WdPostState::Generated,
            },
            WdPostState::Generated => {
                WdPostEvent::Finish => WdPostState::Finished,
            },
        };

        tracing::debug!("wdpost plan: {} -> {}", st, next);

        Ok(next)
    }

    fn exec(
        &self,
        job: &mut Self::Job,
    ) -> Result<Option<Self::Event>, Failure> {
        let inner = WdPost { job };

        match &inner.job.state {
            WdPostState::Empty => inner.acquire(),
            WdPostState::Allocated => inner.generate(),
            WdPostState::Generated => inner.report_result(),
            WdPostState::Finished => return Ok(None),
            WdPostState::Aborted => return Err(TaskAborted.into()),
        }
        .map(Some)
    }

    fn apply(
        &self,
        event: Self::Event,
        state: Self::State,
        job: &mut Self::Job,
    ) -> Result<()> {
        let next = if let WdPostEvent::SetState(s) = event {
            s
        } else {
            state
        };

        if next == job.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        match event {
            WdPostEvent::Idle => {}
            WdPostEvent::SetState(_) => {}
            WdPostEvent::Allocated {
                allocated,
                stop_heartbeat_tx,
            } => {
                job.wdpost_job = Some(allocated);
                job.stop_heartbeat_tx = Some(stop_heartbeat_tx)
            }
            WdPostEvent::Generate(result) => {
                job.wdpost_job_result = Some(result);
            }
            WdPostEvent::Finish => {}
            WdPostEvent::Retry => {}
        }
        tracing::debug!("apply state: {}", next);
        job.state = next;

        Ok(())
    }
}

struct PostPath {
    cache_dir: PathBuf,
    sealed_file: PathBuf,
}

struct WdPost<'a> {
    job: &'a mut WdPostJob,
}

impl WdPost<'_> {
    fn acquire(&self) -> Result<WdPostEvent, Failure> {
        let res = call_rpc!(raw,
            self.job.sealing_ctrl.ctx().global.rpc =>allocate_wdpost_job(
                AllocatePoStSpec {
                    allowed_miners: Some(self.job.sealing_ctrl.config().allowed_miners.clone()),
                    allowed_proof_types: Some(
                        self.job
                            .sealing_ctrl
                            .config()
                            .allowed_proof_types
                            .iter()
                            .flat_map(|x| x.to_post_proofs())
                            .collect()
                    ),
                },
                1,
                self.job.sealing_ctrl.ctx().instance.clone(),
            )
        );

        let mut allocated = match res {
            Ok(a) => a,
            Err(RpcError::JsonRpcError(e))
                if e.code == ErrorCode::MethodNotFound =>
            {
                warn!("damocles-manager may not have enabled the worker-prover module. Please enable the worker-prover module first.");
                return Ok(WdPostEvent::Idle);
            }
            Err(e) => {
                warn!(err=?e, "window PoSt job is not allocated yet, so we can retry even though we got error.");
                return Ok(WdPostEvent::Idle);
            }
        };

        tracing::debug!(allocated = ?allocated.iter().map(|x| &x.id).collect::<Vec<_>>(), "allocated");

        if allocated.is_empty() {
            return Ok(WdPostEvent::Idle);
        }

        let allocated = allocated.swap_remove(0);
        let (stop_heartbeat_tx, stop_heartbeat_rx) = bounded(0);
        Self::start_heartbeat(
            self.job.sealing_ctrl.ctx().global.rpc.clone(),
            allocated.id.clone(),
            self.job.sealing_ctrl.ctx().instance.clone(),
            stop_heartbeat_rx,
        );
        Ok(WdPostEvent::Allocated {
            allocated,
            stop_heartbeat_tx,
        })
    }

    fn generate(&self) -> Result<WdPostEvent, Failure> {
        let _token = self
            .job
            .sealing_ctrl
            .ctrl_ctx()
            .wait(STAGE_NAME_WINDOW_POST)
            .crit()?;

        let wdpost_job = self
            .job
            .wdpost_job
            .as_ref()
            .context("wdpost info not found")
            .abort()?;

        let mut paths = self
            .build_paths(&wdpost_job.input)
            .context("build paths")
            .abort()?;

        // get sealed path and cache path
        let replica = wdpost_job
            .input
            .sectors
            .iter()
            .map(|sector| {
                let p = paths.remove(&sector.sector_id).context("get path")?;
                let replica = PoStReplicaInfo {
                    sector_id: sector.sector_id,
                    comm_r: sector.comm_r,
                    cache_dir: p.cache_dir,
                    sealed_file: p.sealed_file,
                };
                Ok(replica)
            })
            .collect::<Result<Vec<_>>>()
            .abort()?;

        let post_in = WindowPoSt {
            miner_id: wdpost_job.input.miner_id,
            proof_type: wdpost_job.input.proof_type,
            replicas: replica,
            seed: wdpost_job.input.seed,
        };
        let res = self
            .job
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .window_post
            .process(self.job.sealing_ctrl.ctrl_ctx(), post_in);
        if let Err(e) = &res {
            tracing::error!(err=?e, job_id=wdpost_job.id,"wdpost error");
        }
        Ok(WdPostEvent::Generate(res.map_err(|e| e.to_string())))
    }

    fn report_result(&self) -> Result<WdPostEvent, Failure> {
        let job_id = self
            .job
            .wdpost_job
            .as_ref()
            .context("wdpost job cannot be empty")
            .abort()?
            .id
            .clone();
        let result = self
            .job
            .wdpost_job_result
            .as_ref()
            .context("wdpost job result cannot be empty")
            .abort()?;

        let (out, error_reason) = match result {
            Ok(out) => (Some(out.clone()), String::new()),
            Err(err) => (None, err.escape_debug().to_string()),
        };

        call_rpc!(self.job.sealing_ctrl.ctx().global.rpc => wdpost_finish(job_id, out, error_reason,))?;
        if let Some(tx) = &self.job.stop_heartbeat_tx {
            let _ = tx.send(());
        }
        Ok(WdPostEvent::Finish)
    }

    fn build_paths(
        &self,
        input: &WdPoStInput,
    ) -> Result<HashMap<SectorId, PostPath>> {
        struct Resources {
            cache_dir: Vec<Resource>,
            sealed_file: Vec<Resource>,
        }
        let mut resources_by_store = HashMap::new();
        for sector in input.sectors.iter() {
            let sid = SectorID {
                miner: input.miner_id,
                number: sector.sector_id.into(),
            };
            resources_by_store
                .entry(sector.accesses.sealed_file.clone())
                .or_insert(Resources {
                    cache_dir: vec![],
                    sealed_file: vec![],
                })
                .sealed_file
                .push(if sector.upgrade {
                    Resource::Update(sid.clone())
                } else {
                    Resource::Sealed(sid.clone())
                });
            resources_by_store
                .entry(sector.accesses.cache_dir.clone())
                .or_insert(Resources {
                    cache_dir: vec![],
                    sealed_file: vec![],
                })
                .cache_dir
                .push(if sector.upgrade {
                    Resource::UpdateCache(sid.clone())
                } else {
                    Resource::Cache(sid)
                });
        }
        let mut paths: HashMap<SectorId, PostPath> = HashMap::new();
        for (ins, resources) in resources_by_store {
            let instance = self
                .job
                .sealing_ctrl
                .ctx()
                .global
                .attached
                .get(&ins)
                .with_context(|| {
                    format!("get access store instance named {}", ins)
                })?;
            let sealed_files = instance
                .paths(resources.sealed_file.clone())
                .with_context(|| format!("get sealed paths for {}", ins))?;
            let cache_dirs = instance
                .paths(resources.cache_dir.clone())
                .with_context(|| format!("get cache paths for {}", ins))?;
            for (resource, sealed_file) in
                resources.sealed_file.iter().zip(sealed_files)
            {
                let sid = resource
                    .sector_id()
                    .expect("sector_id must be set")
                    .number
                    .into();
                paths.entry(sid).and_modify(|e| {
                    e.sealed_file = sealed_file;
                });
            }
            for (resource, cache_dir) in
                resources.cache_dir.iter().zip(cache_dirs)
            {
                let sid = resource
                    .sector_id()
                    .expect("sector_id must be set")
                    .number
                    .into();
                paths.entry(sid).and_modify(|e| {
                    e.cache_dir = cache_dir;
                });
            }
        }
        Ok(paths)
    }

    fn start_heartbeat(
        rpc: Arc<crate::rpc::sealer::SealerClient>,
        job_id: String,
        worker_name: String,
        stop_rx: Receiver<()>,
    ) {
        let handle = Handle::current();
        std::thread::spawn(move || loop {
            let _guard = handle.enter();

            crossbeam_channel::select! {
                recv(stop_rx) -> _ => break,
                default(Duration::from_secs(3)) => {
                    let worker_name = worker_name.clone();
                    let job_ids = vec![job_id.clone()];
                    let res = call_rpc!(raw, rpc => wdpost_heartbeat(job_ids, worker_name,));
                    match res {
                        Ok(_) => {}
                        Err(RpcError::JsonRpcError(e)) if e.code == ErrorCode::MethodNotFound => {
                            warn!(err=?e, "damocles-manager may not have enabled the worker-prover module. Please enable the worker-prover module first.");
                        }
                        Err(e) =>  {
                            warn!(err=?e, job_id=job_id, "failed to send heartbeat")
                        }
                    }
                    tracing::debug!(job_id = job_id, "send heartbeat");
                }
            }
        });
    }
}
