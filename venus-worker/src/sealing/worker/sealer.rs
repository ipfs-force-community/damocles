use std::fs::{create_dir_all, remove_dir_all, remove_file, OpenOptions};
use std::io::{self, prelude::*};
use std::os::unix::fs::symlink;
use std::path::PathBuf;
use std::thread::sleep;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use async_std::task::block_on;

use crate::logging::{debug, debug_field, debug_span, info, info_span, warn};
use crate::metadb::{rocks::RocksMeta, MetaDocumentDB, MetaError, PrefixedMetaDB};
use crate::rpc::sealer::{
    AcquireDealsSpec, AllocateSectorSpec, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo,
    ReportStateReq, SectorFailure, SectorID, SectorStateChange, SubmitResult, WorkerIdentifier,
};
use crate::sealing::processor::{
    add_piece, clear_cache, seal_commit_phase1, tree_d_path_in_dir, C2Input, PC1Input, PC2Input,
    PaddedBytesAmount, Stage, TreeDInput, UnpaddedBytesAmount,
};
use crate::store::Store;
use crate::watchdog::Ctx;

use super::*;

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

macro_rules! call_rpc {
    ($client:expr, $method:ident, $($arg:expr,)*) => {
        block_on($client.$method(
            $(
                $arg,
            )*
        )).map_err(|e| anyhow!("rpc error: {:?}", e).temp())
    };
}

macro_rules! fetch_cloned_field {
    ($target:expr, $first_field:ident$(.$field:ident)*) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .cloned()
            .ok_or(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            ).abort())
    };

    ($target:expr, $first_field:ident$(.$field:ident)*,) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .cloned()
            .ok_or(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            ).abort())
    };

    ($target:expr) => {
        $target.as_ref().cloned().ok_or(anyhow!(
            "field {} is required",
            stringify!($target)
        ).abort())
    };
}

macro_rules! fetch_field {
    ($target:expr, $first_field:ident$(.$field:ident)*) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .ok_or(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            ).abort())
    };

    ($target:expr, $first_field:ident$(.$field:ident)*,) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .ok_or(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            ).abort())
    };

    ($target:expr) => {
        $target.as_ref().ok_or(anyhow!(
            "field {} is required",
            stringify!($target)
        ).abort())
    };
}

pub struct Sealer<'c> {
    sector: Sector,
    _trace: Vec<Trace>,

    ctx: &'c Ctx,
    ctrl_ctx: &'c CtrlCtx,
    store: &'c Store,
    ident: WorkerIdentifier,

    sector_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
    _trace_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
}

impl<'c> Sealer<'c> {
    pub fn build(ctx: &'c Ctx, ctrl_ctx: &'c CtrlCtx, s: &'c Store) -> Result<Self, Failure> {
        let sector_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_META_PREFIX, &s.meta));

        let sector: Sector = sector_meta
            .get(SECTOR_INFO_KEY)
            .or_else(|e| match e {
                MetaError::NotFound => {
                    let empty = Default::default();
                    sector_meta.set(SECTOR_INFO_KEY, &empty)?;
                    Ok(empty)
                }

                MetaError::Failure(ie) => Err(ie),
            })
            .crit()?;

        let trace_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_TRACE_PREFIX, &s.meta));

        Ok(Sealer {
            sector,
            _trace: Vec::with_capacity(16),

            ctx,
            ctrl_ctx,
            store: s,
            ident: WorkerIdentifier {
                instance: ctx.instance.clone(),
                location: s.location.to_pathbuf(),
            },

            sector_meta,
            _trace_meta: trace_meta,
        })
    }

    fn report_state(
        &self,
        state_change: SectorStateChange,
        fail: Option<SectorFailure>,
    ) -> Result<(), Failure> {
        let sector_id = match self
            .sector
            .base
            .as_ref()
            .map(|base| base.allocated.id.clone())
        {
            Some(sid) => sid,
            None => return Ok(()),
        };

        let _ = call_rpc! {
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
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let _ = call_rpc! {
            self.ctx.global.rpc,
            report_finalized,
            sector_id,
        }?;

        Ok(())
    }

    fn interruptted(&self) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into_failure())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into_failure())
            }

            default => {
                Ok(())
            }
        }
    }

    fn wait_or_interruptted(&self, duration: Duration) -> Result<(), Failure> {
        select! {
            recv(self.ctx.done) -> _done_res => {
                Err(Interrupt.into_failure())
            }

            recv(self.ctrl_ctx.pause_rx) -> pause_res => {
                pause_res.context("pause signal channel closed unexpectedly").crit()?;
                Err(Interrupt.into_failure())
            }

            default(duration) => {
                Ok(())
            }
        }
    }

    pub fn seal(mut self, mut event: Option<Event>) -> Result<(), Failure> {
        loop {
            let event_desc = format!("{:?}", &event);

            let span = info_span!(
                "seal",
                miner = debug_field(self.sector.base.as_ref().map(|b| b.allocated.id.miner)),
                sector = debug_field(self.sector.base.as_ref().map(|b| b.allocated.id.number)),
                event = event_desc.as_str(),
            );

            let enter = span.enter();

            let prev = self.sector.state;
            let is_empty = match self.sector.base.as_ref() {
                None => true,
                Some(base) => {
                    if unsafe { self.ctrl_ctx.sector_id.as_ptr().as_ref() }
                        .and_then(|inner| inner.as_ref())
                        .is_none()
                    {
                        // set sector id for the first time
                        self.ctrl_ctx.sector_id.store(Some(format!(
                            "m-{}-s-{}",
                            base.allocated.id.miner, base.allocated.id.number
                        )));
                    }
                    false
                }
            };

            let handle_res = self.handle(event.take());
            if is_empty {
                match self.sector.base.as_ref() {
                    Some(base) => {
                        self.ctrl_ctx.sector_id.store(Some(format!(
                            "m-{}-s-{}",
                            base.allocated.id.miner, base.allocated.id.number
                        )));
                    }

                    None => {}
                };
            } else {
                if self.sector.base.is_none() {
                    self.ctrl_ctx.sector_id.store(None);
                }
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
                    event: event_desc,
                },
                fail,
            ) {
                error!("report state failed: {:?}", rerr);
            };

            match handle_res {
                Ok(Some(evt)) => {
                    event.replace(evt);
                }

                Ok(None) => {
                    if let Err(rerr) = self.report_finalized() {
                        error!("report finalized failed: {:?}", rerr);
                    }

                    self.finalize()?;
                    return Ok(());
                }

                Err(Failure(Level::Abort, aerr)) => {
                    if let Err(rerr) = self.report_finalized() {
                        error!("report aborted sector finalized failed: {:?}", rerr);
                    }

                    warn!("cleanup aborted sector");
                    self.finalize()?;
                    return Err(aerr.abort());
                }

                Err(Failure(Level::Temporary, terr)) => {
                    if self.sector.retry >= self.store.config.max_retries {
                        // reset retry times;
                        self.sync(|s| {
                            s.retry = 0;
                            Ok(())
                        })?;

                        return Err(terr.perm());
                    }

                    self.sync(|s| {
                        warn!(retry = s.retry, "temp error occurred: {:?}", terr,);

                        s.retry += 1;

                        Ok(())
                    })?;

                    info!(
                        interval = debug_field(self.store.config.recover_interval),
                        "wait before recovering"
                    );
                    sleep(self.store.config.recover_interval)
                }

                Err(f) => return Err(f),
            }

            drop(enter);
        }
    }

    fn sync<F: FnOnce(&mut Sector) -> Result<()>>(&mut self, modify_fn: F) -> Result<(), Failure> {
        modify_fn(&mut self.sector).crit()?;
        self.sector_meta.set(SECTOR_INFO_KEY, &self.sector).crit()
    }

    fn finalize(self) -> Result<(), Failure> {
        self.store.cleanup().crit()?;
        self.sector_meta.remove(SECTOR_INFO_KEY).crit()?;
        Ok(())
    }

    fn sector_path(&self, sector_id: &SectorID) -> String {
        format!("s-{}-{}", sector_id.miner, sector_id.number)
    }

    fn prepared_dir(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join("prepared")
            .join(self.sector_path(sector_id))
    }

    fn cache_dir(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join("cache")
            .join(self.sector_path(sector_id))
    }

    fn sealed_file(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join("sealed")
            .join(self.sector_path(sector_id))
    }

    fn staged_file(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join("unsealed")
            .join(self.sector_path(sector_id))
    }

    fn handle(&mut self, event: Option<Event>) -> Result<Option<Event>, Failure> {
        self.interruptted()?;

        let prev = self.sector.state;

        if let Some(evt) = event {
            match evt {
                Event::Retry => {
                    debug!(
                        prev = debug_field(self.sector.state),
                        sleep = debug_field(self.store.config.recover_interval),
                        "Event::Retry captured"
                    );

                    sleep(self.store.config.recover_interval);
                }

                other => {
                    self.sync(move |s| other.apply(s))?;
                }
            };
        };

        let span = info_span!(
            "handle",
            prev = debug_field(prev),
            current = debug_field(self.sector.state),
        );

        let _enter = span.enter();

        self.ctrl_ctx.sealing_state.store(self.sector.state);

        debug!("handling");

        match self.sector.state {
            State::Empty => self.handle_empty(),

            State::Allocated => self.handle_allocated(),

            State::DealsAcquired => self.handle_deals_acquired(),

            State::PieceAdded => self.handle_piece_added(),

            State::TreeDBuilt => self.handle_tree_d_built(),

            State::TicketAssigned => self.handle_ticket_assigned(),

            State::PC1Done => self.handle_pc1_done(),

            State::PC2Done => self.handle_pc2_done(),

            State::PCSubmitted => self.handle_pc_submitted(),

            State::PCLanded => self.handle_pc_landed(),

            State::Persisted => self.handle_persisted(),

            State::PersistanceSubmitted => self.handle_persistance_submitted(),

            State::SeedAssigned => self.handle_seed_assigned(),

            State::C1Done => self.handle_c1_done(),

            State::C2Done => self.handle_c2_done(),

            State::ProofSubmitted => self.handle_proof_submitted(),

            State::Finished => return Ok(None),

            State::Aborted => {
                warn!("sector aborted");
                return Ok(None);
            }
        }
        .map(From::from)
    }

    fn handle_empty(&mut self) -> HandleResult {
        let maybe_allocated_res = call_rpc! {
            self.ctx.global.rpc,
            allocate_sector,
            AllocateSectorSpec {
                allowed_miners: None,
                allowed_proof_types: None,
            },
        };

        let maybe_allocated = match maybe_allocated_res {
            Ok(a) => a,
            Err(e) => {
                warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Retry);
            }
        };

        let sector = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Retry),
        };

        // init required dirs & files
        let cache_dir = self.cache_dir(&sector.id);
        create_dir_all(&cache_dir)
            .context("init cache dir")
            .crit()?;

        self.staged_file(&sector.id)
            .parent()
            .map(|dir| create_dir_all(dir))
            .transpose()
            .crit()?;

        self.sealed_file(&sector.id)
            .parent()
            .map(|dir| create_dir_all(dir))
            .transpose()
            .crit()?;

        // let mut opt = OpenOptions::new();
        // opt.create(true).read(true).write(true).truncate(true);

        // {
        //     let staged_file = opt.open(self.staged_file(&sector.id)).crit()?;
        //     drop(staged_file);
        // }

        // {
        //     let sealed_file = opt.open(self.sealed_file(&sector.id)).crit()?;
        //     drop(sealed_file);
        // }

        Ok(Event::Allocate(sector))
    }

    fn handle_allocated(&mut self) -> HandleResult {
        if !self.store.config.enable_deals {
            return Ok(Event::AcquireDeals(None));
        }

        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let deals = call_rpc! {
            self.ctx.global.rpc,
            acquire_deals,
            sector_id,
            AcquireDealsSpec {
                max_deals: None,
            },
        }?;

        Ok(Event::AcquireDeals(deals))
    }

    fn handle_deals_acquired(&mut self) -> HandleResult {
        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let mut staged_file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            // to make sure that we won't write into the staged file with any data exists
            .truncate(true)
            .open(self.staged_file(sector_id))
            .abort()?;

        // TODO: this is only for pledged sector
        let proof_type = fetch_cloned_field! {
            self.sector.base,
            allocated.proof_type,
        }?;

        let sector_size = proof_type.sector_size();
        let unpadded_size: UnpaddedBytesAmount = PaddedBytesAmount(sector_size).into();

        let mut pledge_piece = io::repeat(0).take(unpadded_size.0);
        let (piece_info, _) = add_piece(
            proof_type.into(),
            &mut pledge_piece,
            &mut staged_file,
            unpadded_size,
            &[],
        )
        .abort()?;

        Ok(Event::AddPiece(vec![piece_info]))
    }

    fn handle_piece_added(&mut self) -> HandleResult {
        // TODO: handle static tree_d file for cc sectors
        let token = self.ctx.global.limit.acquire(Stage::TreeD).crit()?;

        let proof_type = fetch_cloned_field! {
            self.sector.base,
            allocated.proof_type,
        }?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let prepared_dir = self.prepared_dir(sector_id);
        create_dir_all(&prepared_dir).crit()?;

        let tree_d_path = tree_d_path_in_dir(&prepared_dir);
        if tree_d_path.exists() {
            remove_file(&tree_d_path)
                .with_context(|| format!("cleanup preprared tree d file {:?}", tree_d_path))
                .crit()?;
        }

        if let Some(static_tree_path) = self.ctx.global.static_tree_d.get(&proof_type.sector_size())
        {
            symlink(static_tree_path, tree_d_path_in_dir(&prepared_dir)).crit()?;
            return Ok(Event::BuildTreeD);
        }

        let staged_file = self.staged_file(sector_id);

        self.ctx
            .global
            .processors
            .tree_d
            .process(TreeDInput {
                registered_proof: proof_type.into(),
                staged_file,
                cache_dir: prepared_dir,
            })
            .abort()?;

        drop(token);
        Ok(Event::BuildTreeD)
    }

    fn handle_tree_d_built(&mut self) -> HandleResult {
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let ticket = call_rpc! {
            self.ctx.global.rpc,
            assign_ticket,
            sector_id,
        }?;

        Ok(Event::AssignTicket(ticket))
    }

    fn cleanup_before_pc1(&self, cache_dir: &PathBuf, sealed_file: &PathBuf) -> Result<()> {
        // TODO: see if we have more graceful ways to handle restarting pc1
        remove_dir_all(&cache_dir).and_then(|_| create_dir_all(&cache_dir))?;
        debug!("init cache dir {:?} before pc1", cache_dir);

        let empty_sealed_file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(true)
            .open(&sealed_file)?;
        debug!("truncate sealed file {:?} before pc1", sealed_file);
        drop(empty_sealed_file);

        Ok(())
    }

    fn handle_ticket_assigned(&mut self) -> HandleResult {
        let token = self.ctx.global.limit.acquire(Stage::PC1).crit()?;

        let proof_type = fetch_cloned_field! {
            self.sector.base,
            allocated.proof_type,
        }?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let ticket = fetch_cloned_field! {
            self.sector.phases.ticket,
            ticket,
        }?;

        let piece_infos = fetch_cloned_field! {
            self.sector.phases.pieces
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let staged_file = self.staged_file(sector_id);
        let sealed_file = self.sealed_file(sector_id);
        let prepared_dir = self.prepared_dir(sector_id);

        self.cleanup_before_pc1(&cache_dir, &sealed_file).crit()?;
        symlink(
            tree_d_path_in_dir(&prepared_dir),
            tree_d_path_in_dir(&cache_dir),
        )
        .crit()?;

        let (prover_id, sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let out = self
            .ctx
            .global
            .processors
            .pc1
            .process(PC1Input {
                registered_proof: proof_type.into(),
                cache_path: cache_dir,
                in_path: staged_file,
                out_path: sealed_file,
                prover_id,
                sector_id,
                ticket: ticket.0,
                piece_infos,
            })
            .abort()?;

        drop(token);
        Ok(Event::PC1(out))
    }

    fn cleanup_before_pc2(&self, cache_dir: &PathBuf) -> Result<()> {
        for entry_res in cache_dir.read_dir()? {
            let entry = entry_res?;
            let fname = entry.file_name();
            if let Some(fname_str) = fname.to_str() {
                let should = fname_str == "p_aux"
                    || fname_str == "t_aux"
                    || fname_str.contains("tree-c")
                    || fname_str.contains("tree-r-last");

                if !should {
                    continue;
                }

                let p = entry.path();
                remove_file(&p).with_context(|| format!("remove cached file {:?}", p))?;
                debug!("remove cached file {:?} before pc2", p);
            }
        }

        Ok(())
    }

    fn handle_pc1_done(&mut self) -> HandleResult {
        let token = self.ctx.global.limit.acquire(Stage::PC2).crit()?;

        let pc1out = fetch_cloned_field! {
            self.sector.phases.pc1out
        }?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        self.cleanup_before_pc2(&cache_dir).crit()?;

        let out = self
            .ctx
            .global
            .processors
            .pc2
            .process(PC2Input {
                pc1out,
                cache_dir,
                sealed_file,
            })
            .abort()?;

        drop(token);
        Ok(Event::PC2(out))
    }

    fn handle_pc2_done(&mut self) -> HandleResult {
        let sector = fetch_cloned_field! {
            self.sector.base,
            allocated,
        }?;

        let deals = self
            .sector
            .deals
            .as_ref()
            .map(|d| d.iter().map(|i| i.id).collect())
            .unwrap_or(vec![]);

        let pinfo = PreCommitOnChainInfo {
            comm_r: fetch_cloned_field! {
                self.sector.phases.pc2out,
                comm_r,
            }?,
            comm_d: fetch_cloned_field! {
                self.sector.phases.pc2out,
                comm_d,
            }?,
            ticket: fetch_cloned_field! {
                self.sector.phases.ticket
            }?,
            deals,
        };

        let res = call_rpc! {
            self.ctx.global.rpc,
            submit_pre_commit,
            sector,
            pinfo,
            self.sector.phases.pc2_re_submit,
        }?;

        // TODO: handle submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitPC),

            SubmitResult::MismatchedSubmission => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
            }

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),
        }
    }

    fn handle_pc_submitted(&mut self) -> HandleResult {
        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        'POLL: loop {
            let state = call_rpc! {
                self.ctx.global.rpc,
                poll_pre_commit_state,
                sector_id.clone(),
            }?;

            match state.state {
                OnChainState::Landed => break 'POLL,
                OnChainState::NotFound => {
                    return Err(anyhow!("pre commit on-chain info not found").perm())
                }

                OnChainState::Failed => {
                    warn!("pre commit on-chain info failed: {:?}", state.desc);
                    // TODO: make it configurable
                    self.wait_or_interruptted(Duration::from_secs(30))?;
                    return Ok(Event::ReSubmitPC);
                }

                OnChainState::PermFailed => {
                    return Err(anyhow!(
                        "pre commit on-chain info permanent failed: {:?}",
                        state.desc
                    )
                    .perm())
                }

                OnChainState::Pending | OnChainState::Packed => {}
            }

            debug!(
                state = debug_field(state.state),
                interval = debug_field(self.store.config.rpc_polling_interval),
                "waiting for next round of polling pre commit state",
            );

            self.wait_or_interruptted(self.store.config.rpc_polling_interval)?;
        }

        debug!("pre commit landed");

        Ok(Event::CheckPC)
    }

    fn handle_pc_landed(&mut self) -> HandleResult {
        let allocated = fetch_field! {
            self.sector.base,
            allocated,
        }?;

        let sector_id = &allocated.id;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let mut wanted = vec![sealed_file];

        // here we treat fs err as temp
        for entry_res in cache_dir.read_dir().temp()? {
            let entry = entry_res.temp()?;
            let fname = entry.file_name();
            if let Some(fname_str) = fname.to_str() {
                let should = fname_str == "p_aux"
                    || fname_str == "t_aux"
                    || fname_str.contains("tree-r-last");

                if !should {
                    continue;
                }

                wanted.push(entry.path());
            }
        }

        let mut opt = OpenOptions::new();
        opt.read(true);

        for one in wanted {
            let target_path = match one.strip_prefix(&self.store.data_path) {
                Ok(p) => p,
                Err(e) => {
                    return Err(e.crit()).context(format!(
                        "strip prefix {:?} for {:?}",
                        self.store.data_path, one
                    ));
                }
            };

            let copy_span = debug_span!(
                "persist",
                src = debug_field(&one),
                dst = debug_field(&target_path),
            );

            let copy_enter = copy_span.enter();

            let source = opt.open(&one).crit()?;
            let size = self
                .ctx
                .global
                .remote_store
                .put(target_path, Box::new(source))
                .crit()?;

            debug!(size, "persist done");

            drop(copy_enter);
        }

        Ok(Event::Persist(self.ctx.global.remote_store.instance()))
    }

    fn handle_persisted(&mut self) -> HandleResult {
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let instance = fetch_cloned_field! {
            self.sector.phases.persist_instance
        }?;

        let checked = call_rpc! {
            self.ctx.global.rpc,
            submit_persisted,
            sector_id,
            instance,
        }?;

        if checked {
            Ok(Event::SubmitPersistance)
        } else {
            Err(anyhow!(
                "sector files are persisted but unavailable for sealer"
            ))
            .perm()
        }
    }

    fn handle_persistance_submitted(&mut self) -> HandleResult {
        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let seed = loop {
            let wait = call_rpc! {
                self.ctx.global.rpc,
                wait_seed,
                sector_id.clone(),
            }?;

            if let Some(seed) = wait.seed {
                break seed;
            };

            if !wait.should_wait || wait.delay == 0 {
                return Err(anyhow!("invalid empty wait_seed response").temp());
            }

            let delay = Duration::from_secs(wait.delay);

            debug!(
                delay = debug_field(delay),
                "waiting for next round of polling seed"
            );

            self.wait_or_interruptted(delay)?;
        };

        Ok(Event::AssignSeed(seed))
    }

    fn handle_seed_assigned(&mut self) -> HandleResult {
        let token = self.ctx.global.limit.acquire(Stage::C1).crit()?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let (seal_prover_id, seal_sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let piece_infos = fetch_field! {
            self.sector.phases.pieces
        }?;

        let ticket = fetch_cloned_field! {
            self.sector.phases.ticket,
            ticket,
        }?;

        let seed = fetch_cloned_field! {
            self.sector.phases.seed,
            seed,
        }?;

        let p2out = fetch_cloned_field! {
            self.sector.phases.pc2out
        }?;

        let out = seal_commit_phase1(
            cache_dir,
            sealed_file,
            seal_prover_id,
            seal_sector_id,
            ticket.0,
            seed.0,
            p2out,
            piece_infos,
        )
        .perm()?;

        drop(token);
        Ok(Event::C1(out))
    }

    fn handle_c1_done(&mut self) -> HandleResult {
        let token = self.ctx.global.limit.acquire(Stage::C2).crit()?;

        let c1out = fetch_cloned_field! {
            self.sector.phases.c1out
        }?;

        let (prover_id, sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let out = self
            .ctx
            .global
            .processors
            .c2
            .process(C2Input {
                c1out,
                prover_id,
                sector_id,
            })
            .perm()?;

        drop(token);
        Ok(Event::C2(out))
    }

    fn handle_c2_done(&mut self) -> HandleResult {
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let info = ProofOnChainInfo {
            proof: fetch_cloned_field! {
                self.sector.phases.c2out,
                proof,
            }?
            .into(),
        };

        let res = call_rpc! {
            self.ctx.global.rpc,
            submit_proof,
            sector_id,
            info,
            self.sector.phases.c2_re_submit,
        }?;

        // TODO: submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitProof),

            SubmitResult::MismatchedSubmission => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm())
            }

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).perm()),
        }
    }

    fn handle_proof_submitted(&mut self) -> HandleResult {
        let allocated = fetch_field! {
            self.sector.base,
            allocated,
        }?;

        let sector_id = &allocated.id;

        if !self.store.config.ignore_proof_check {
            'POLL: loop {
                let state = call_rpc! {
                    self.ctx.global.rpc,
                    poll_proof_state,
                    sector_id.clone(),
                }?;

                match state.state {
                    OnChainState::Landed => break 'POLL,
                    OnChainState::NotFound => {
                        return Err(anyhow!("proof on-chain info not found").perm())
                    }

                    OnChainState::Failed => {
                        warn!("proof on-chain info failed: {:?}", state.desc);
                        // TODO: make it configurable
                        self.wait_or_interruptted(Duration::from_secs(30))?;
                        return Ok(Event::ReSubmitProof);
                    }

                    OnChainState::PermFailed => {
                        return Err(anyhow!(
                            "proof on-chain info permanent failed: {:?}",
                            state.desc
                        )
                        .perm())
                    }

                    OnChainState::Pending | OnChainState::Packed => {}
                }

                debug!(
                    state = debug_field(state.state),
                    interval = debug_field(self.store.config.rpc_polling_interval),
                    "waiting for next round of polling proof state",
                );

                self.wait_or_interruptted(self.store.config.rpc_polling_interval)?;
            }
        }

        let cache_dir = self.cache_dir(sector_id);
        let sector_size = allocated.proof_type.sector_size();

        // we should be careful here, use failure as temporary
        clear_cache(sector_size, &cache_dir).temp()?;
        debug!(
            dir = debug_field(&cache_dir),
            "clean up unnecessary cached files"
        );

        Ok(Event::Finish)
    }
}
