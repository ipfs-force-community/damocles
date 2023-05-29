use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use crossbeam_channel::select;
use forest_cid::json::CidJson;

use self::planner::default_plan;

use super::{super::failure::*, CtrlCtx};
use crate::logging::{debug, error, info, warn, warn_span};
use crate::metadb::{rocks::RocksMeta, MaybeDirty, MetaDocumentDB, PrefixedMetaDB, Saved};
use crate::rpc::sealer::{ReportStateReq, SectorFailure, SectorID, SectorStateChange, WorkerIdentifier};
use crate::sealing::config::Config;
use crate::store::Store;
use crate::types::SealProof;
use crate::watchdog::Ctx;

pub mod event;
use event::*;

pub mod sector;
use sector::*;

mod planner;
use planner::{get_planner, Planner};

mod entry;
use entry::*;

#[macro_use]
mod util;
use util::*;

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

pub struct Task<'c> {
    sector: Saved<Sector, &'static str, PrefixedMetaDB<&'c RocksMeta>>,
    _trace: Vec<Trace>,

    ctx: &'c Ctx,
    ctrl_ctx: &'c CtrlCtx,
    sealing_config: &'c Config,
    store: &'c Store,
    ident: WorkerIdentifier,

    _trace_meta: MetaDocumentDB<PrefixedMetaDB<&'c RocksMeta>>,
}

// properties
impl<'c> Task<'c> {
    fn sector_id(&self) -> Result<&SectorID, Failure> {
        field_required! {
            sector_id,
            self.sector.base.as_ref().map(|b| &b.allocated.id)
        }

        Ok(sector_id)
    }

    fn sector_proof_type(&self) -> Result<&SealProof, Failure> {
        field_required! {
            proof_type,
            self.sector.base.as_ref().map(|b| &b.allocated.proof_type)
        }

        Ok(proof_type)
    }
}

// public methods
impl<'c> Task<'c> {
    pub fn build(ctx: &'c Ctx, ctrl_ctx: &'c CtrlCtx, sealing_config: &'c mut Config, s: &'c mut Store) -> Result<Self, Failure> {
        let sector_meta = PrefixedMetaDB::wrap(SECTOR_META_PREFIX, &s.meta);
        let mut sector: Saved<Sector, _, _> = Saved::load(SECTOR_INFO_KEY, sector_meta).context("load sector").crit()?;

        sealing_config
            .reload_if_needed(|_, _| Ok(true))
            .context("reload sealing thread hot config")
            .crit()?;

        if &sector.plan != sealing_config.plan() {
            // init setup sector plan or modify by hot config
            sector.plan = sealing_config.plan().clone();
        }

        // create sector or sync sector plan
        sector.sync().context("init sync sector").crit()?;

        ctrl_ctx
            .update_state(|cst| cst.job.plan = sector.plan.clone().unwrap_or_else(|| default_plan().to_owned()))
            .context("update ctrl state")
            .perm()?;

        let trace_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_TRACE_PREFIX, &s.meta));

        Ok(Task {
            sector,
            _trace: Vec::with_capacity(16),

            ctx,
            ctrl_ctx,
            sealing_config,
            store: s,
            ident: WorkerIdentifier {
                instance: ctx.instance.clone(),
                location: s.location.to_pathbuf(),
            },

            _trace_meta: trace_meta,
        })
    }

    fn report_state(&self, state_change: SectorStateChange, fail: Option<SectorFailure>) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.ctx.global.rpc,
            report_state,
            sector_id,
            ReportStateReq {
                worker: self.ident.clone(),
                state_change,
                failure: fail,
            },
        }?;

        Ok(())
    }

    fn report_finalized(&self) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.ctx.global.rpc,
            report_finalized,
            sector_id,
        }?;

        Ok(())
    }

    fn report_aborted(&self, reason: String) -> Result<(), Failure> {
        let sector_id = match self.sector.base.as_ref().map(|base| base.allocated.id.clone()) {
            Some(sid) => sid,
            None => return Ok(()),
        };

        call_rpc! {
            self.ctx.global.rpc,
            report_aborted,
            sector_id,
            reason,
        }?;

        Ok(())
    }

    fn interrupted(&self) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into())
            }

            default => {
                Ok(())
            }
        }
    }

    fn wait_or_interrupted(&self, duration: Duration) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into())
            }

            default(duration) => {
                Ok(())
            }
        }
    }

    pub fn exec(mut self, state: Option<State>) -> Result<(), Failure> {
        let mut event = state.map(Event::SetState);
        let mut task_idle_count = 0;
        loop {
            let span = warn_span!(
                "seal",
                miner = ?self.sector.base.as_ref().map(|b| b.allocated.id.miner),
                sector = ?self.sector.base.as_ref().map(|b| b.allocated.id.number),
                ?event,
            );

            let _enter = span.enter();

            let prev = self.sector.state;
            let is_empty = match self.sector.base.as_ref() {
                None => true,
                Some(base) => {
                    self.ctrl_ctx
                        .update_state(|cst| {
                            cst.job.id.replace(base.allocated.id.clone());
                        })
                        .crit()?;
                    false
                }
            };

            let handle_res = self.handle(event.take());
            if is_empty {
                if let Some(base) = self.sector.base.as_ref() {
                    self.ctrl_ctx
                        .update_state(|cst| {
                            cst.job.id.replace(base.allocated.id.clone());
                        })
                        .crit()?;
                }
            } else if self.sector.base.is_none() {
                self.ctrl_ctx
                    .update_state(|cst| {
                        cst.job.id.take();
                    })
                    .crit()?;
            }

            let fail = if let Err(eref) = handle_res.as_ref() {
                Some(SectorFailure {
                    level: format!("{:?}", eref.0),
                    desc: format!("{:?}", eref.1),
                })
            } else {
                None
            };

            if let Err(rerr) = self.report_state(
                SectorStateChange {
                    prev: prev.as_str().to_owned(),
                    next: self.sector.state.as_str().to_owned(),
                    event: format!("{:?}", event),
                },
                fail,
            ) {
                error!("report state failed: {:?}", rerr);
            };

            match handle_res {
                Ok(Some(evt)) => {
                    if let Event::Idle = evt {
                        task_idle_count += 1;
                        if task_idle_count > self.sealing_config.request_task_max_retries {
                            info!(
                                "The task has returned `Event::Idle` for more than {} times. break the task",
                                self.sealing_config.request_task_max_retries
                            );

                            // when the planner tries to request a task but fails(including no task) for more than
                            // `config::sealing::request_task_max_retries` times, this task is really considered idle,
                            // break this task loop. that we have a chance to reload `sealing_thread` hot config file,
                            // or do something else.

                            if self.sealing_config.check_modified() {
                                // cleanup sector if the hot config modified
                                self.finalize()?;
                            }
                            return Ok(());
                        }
                    }
                    event.replace(evt);
                }

                Ok(None) => match self.report_finalized().context("report finalized") {
                    Ok(_) => {
                        self.finalize()?;
                        return Ok(());
                    }
                    Err(terr) => self.retry(terr.1)?,
                },

                Err(Failure(Level::Abort, aerr)) => {
                    if let Err(rerr) = self.report_aborted(aerr.to_string()) {
                        error!("report aborted sector failed: {:?}", rerr);
                    }

                    warn!("cleanup aborted sector");
                    self.finalize()?;
                    return Err(aerr.abort());
                }

                Err(Failure(Level::Temporary, terr)) => self.retry(terr)?,

                Err(f) => return Err(f),
            }
        }
    }

    fn retry(&mut self, temp_err: anyhow::Error) -> Result<(), Failure> {
        if self.sector.retry >= self.sealing_config.max_retries {
            // reset retry times;
            self.sync(|s| {
                s.retry = 0;
                Ok(())
            })?;

            return Err(temp_err.perm());
        }

        self.sync(|s| {
            warn!(retry = s.retry, "temp error occurred: {:?}", temp_err);

            s.retry += 1;

            Ok(())
        })?;

        info!(
            interval = ?self.sealing_config.recover_interval,
            "wait before recovering"
        );

        self.wait_or_interrupted(self.sealing_config.recover_interval)?;
        Ok(())
    }

    fn sync<F: FnOnce(&mut MaybeDirty<Sector>) -> Result<()>>(&mut self, modify_fn: F) -> Result<(), Failure> {
        modify_fn(self.sector.inner_mut()).crit()?;
        self.sector.sync().context("sync sector").crit()
    }

    fn finalize(mut self) -> Result<(), Failure> {
        self.store.cleanup().context("cleanup store").crit()?;
        self.sector.delete().context("remove sector").crit()
    }

    fn sector_path(&self, sector_id: &SectorID) -> String {
        format!("s-t0{}-{}", sector_id.miner, sector_id.number)
    }

    fn prepared_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(&self.store.data_path, PathBuf::from("prepared").join(self.sector_path(sector_id)))
    }

    fn cache_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(&self.store.data_path, PathBuf::from("cache").join(self.sector_path(sector_id)))
    }

    fn sealed_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("sealed").join(self.sector_path(sector_id)))
    }

    fn staged_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("unsealed").join(self.sector_path(sector_id)))
    }

    fn piece_file(&self, piece_cid: &CidJson) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("unsealed").join(format!("{}", piece_cid.0)))
    }

    fn update_file(&self, sector_id: &SectorID) -> Entry {
        Entry::file(&self.store.data_path, PathBuf::from("update").join(self.sector_path(sector_id)))
    }

    fn update_cache_dir(&self, sector_id: &SectorID) -> Entry {
        Entry::dir(
            &self.store.data_path,
            PathBuf::from("update-cache").join(self.sector_path(sector_id)),
        )
    }

    fn handle(&mut self, event: Option<Event>) -> Result<Option<Event>, Failure> {
        self.interrupted()?;

        let prev = self.sector.state;
        let planner = get_planner(self.sector.plan.as_deref()).perm()?;

        if let Some(evt) = event {
            match evt {
                Event::Idle | Event::Retry => {
                    debug!(
                        prev = ?self.sector.state,
                        sleep = ?self.sealing_config.recover_interval,
                        "Event::{:?} captured", evt
                    );

                    self.wait_or_interrupted(self.sealing_config.recover_interval)?;
                }

                other => {
                    self.sync(|s| other.apply(&planner, s))?;
                }
            };
        };

        let span = warn_span!("handle", ?prev, current = ?self.sector.state);

        let _enter = span.enter();

        self.ctrl_ctx
            .update_state(|cst| {
                let _ = std::mem::replace(&mut cst.job.state, self.sector.state);
            })
            .crit()?;

        debug!("handling");

        planner.exec(self)
    }
}
