use std::fs::{create_dir_all, OpenOptions};
use std::io::{self, prelude::*};
use std::path::PathBuf;
use std::sync::Arc;
use std::thread::sleep;

use anyhow::{anyhow, Context, Result};
use async_std::task::block_on;
use crossbeam_channel::{select, Receiver, TryRecvError};
use filecoin_proofs_api::seal::{
    add_piece, clear_cache, seal_commit_phase1, seal_commit_phase2, seal_pre_commit_phase1,
    seal_pre_commit_phase2,
};
use glob::glob;

use crate::infra::objstore::ObjectStore;
use crate::logging::{debug, debug_field, debug_span, error, info, info_span, warn};
use crate::metadb::{rocks::RocksMeta, MetaDocumentDB, MetaError, PrefixedMetaDB};
use crate::rpc::{
    self, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo, SealerRpcClient, SectorID,
    SubmitResult,
};

use super::event::Event;
use super::failure::*;
use super::resource::Pool;
use super::sector::{PaddedBytesAmount, Sector, State, Trace, UnpaddedBytesAmount};
use super::store::Store;

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

// TODO: with_level, turn std error into leveled failure
// TODO: simpler way to add context to failure

type HandleResult = Result<Event, Failure>;

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

enum Stage {
    PC1,
    PC2,
    C1,
    C2,
}

impl AsRef<str> for Stage {
    fn as_ref(&self) -> &str {
        match self {
            Self::PC1 => "pc1",
            Self::PC2 => "pc2",
            Self::C1 => "c1",
            Self::C2 => "c2",
        }
    }
}

struct Ctx<'c, O> {
    sector: Sector,
    _trace: Vec<Trace>,

    store: &'c Store,
    rpc: Arc<SealerRpcClient>,
    remote_store: Arc<O>,
    limit: Arc<Pool>,

    sector_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
    _trace_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
}

impl<'c, O: ObjectStore> Ctx<'c, O> {
    fn build(
        s: &'c Store,
        rpc: Arc<SealerRpcClient>,
        remote_store: Arc<O>,
        limit: Arc<Pool>,
    ) -> Result<Self, Failure> {
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

        Ok(Ctx {
            sector,
            _trace: Vec::with_capacity(16),

            store: s,
            rpc,
            remote_store,
            limit,

            sector_meta,
            _trace_meta: trace_meta,
        })
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

    fn cache_dir(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join(self.sector_path(sector_id))
            .join("cache")
    }

    fn sealed_file(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join(self.sector_path(sector_id))
            .join("sealed")
    }

    fn staged_file(&self, sector_id: &SectorID) -> PathBuf {
        self.store
            .data_path
            .join(self.sector_path(sector_id))
            .join("staged")
    }

    fn handle(&mut self, event: Option<Event>) -> Result<Option<Event>, Failure> {
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

        match self.sector.state {
            State::Empty => self.handle_empty(),

            State::Allocated => self.handle_allocated(),

            State::DealsAcquired => self.handle_deals_acquired(),

            State::PieceAdded => self.handle_piece_added(),

            State::TicketAssigned => self.handle_ticket_assigned(),

            State::PC1Done => self.handle_pc1_done(),

            State::PC2Done => self.handle_pc2_done(),

            State::PCSubmitted => self.handle_pc_submitted(),

            State::SeedAssigned => self.handle_seed_assigned(),

            State::C1Done => self.handle_c1_done(),

            State::C2Done => self.handle_c2_done(),

            State::Persisted => self.handle_persisted(),

            State::ProofSubmitted => self.handle_proof_submitted(),

            State::Finished => return Ok(None),
        }
        .map(From::from)
    }

    fn handle_empty(&mut self) -> HandleResult {
        let maybe_allocated = call_rpc! {
            self.rpc,
            allocate_sector,
            rpc::AllocateSectorSpec {
                allowed_miners: None,
                allowed_proot_types: None,
            },
        }?;

        let sector = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Retry),
        };

        // init required dirs & files
        let cache_dir = self.cache_dir(&sector.id);
        create_dir_all(&cache_dir).crit()?;

        let mut opt = OpenOptions::new();
        opt.create(true).read(true).write(true).truncate(true);

        {
            let staged_file = opt.open(self.staged_file(&sector.id)).crit()?;
            drop(staged_file);
        }

        {
            let sealed_file = opt.open(self.sealed_file(&sector.id)).crit()?;
            drop(sealed_file);
        }

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
            self.rpc,
            acquire_deals,
            sector_id,
            rpc::AcquireDealsSpec {
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
            .read(true)
            .write(true)
            .open(self.staged_file(sector_id))
            .abort()?;

        // TODO: this is only for pledged sector
        let proof_type = fetch_cloned_field! {
            self.sector.base,
            allocated.proof_type,
        }?;

        let sector_size = proof_type.sector_size();
        let unpadded_size: UnpaddedBytesAmount = PaddedBytesAmount(sector_size.0).into();

        let mut pledge_piece = io::repeat(0).take(unpadded_size.0);
        let (piece_info, _) = add_piece(
            proof_type,
            &mut pledge_piece,
            &mut staged_file,
            unpadded_size,
            &[],
        )
        .abort()?;

        Ok(Event::AddPiece(vec![piece_info]))
    }

    fn handle_piece_added(&mut self) -> HandleResult {
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let ticket = call_rpc! {
            self.rpc,
            assign_ticket,
            sector_id,
        }?;

        Ok(Event::AssignTicket(ticket))
    }

    fn handle_ticket_assigned(&mut self) -> HandleResult {
        let token = self.limit.acquire(Stage::PC1).crit()?;

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

        let piece_infos = fetch_field! {
            self.sector.phases.pieces
        }?;

        let cache_path = self.cache_dir(sector_id);
        let staged_file = self.staged_file(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let (seal_prover_id, seal_sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let out = seal_pre_commit_phase1(
            proof_type,
            cache_path,
            staged_file,
            sealed_file,
            seal_prover_id,
            seal_sector_id,
            ticket.0,
            piece_infos,
        )
        .abort()?;

        drop(token);
        Ok(Event::PC1(out))
    }

    fn handle_pc1_done(&mut self) -> HandleResult {
        let token = self.limit.acquire(Stage::PC2).crit()?;

        let pc1out = fetch_cloned_field! {
            self.sector.phases.pc1out
        }?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let out = seal_pre_commit_phase2(pc1out, cache_dir, sealed_file).abort()?;

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

        let info = PreCommitOnChainInfo {
            comm_r: fetch_cloned_field! {
                self.sector.phases.pc2out,
                comm_r,
            }?,
            ticket: fetch_cloned_field! {
                self.sector.phases.ticket
            }?,
            deals,
        };

        let res = call_rpc! {
            self.rpc,
            submit_pre_commit,
            sector,
            info,
        }?;

        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitPC),

            SubmitResult::MismatchedSubmission => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
            }

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).temp()),
        }
    }

    fn handle_pc_submitted(&mut self) -> HandleResult {
        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        'POLL: loop {
            let state = call_rpc! {
                self.rpc,
                poll_pre_commit_state,
                sector_id.clone(),
            }?;

            match state.state {
                OnChainState::Landed => break 'POLL,
                OnChainState::NotFound => {
                    return Err(anyhow!("pre commit on-chain info not found").abort())
                }
                OnChainState::Pending | OnChainState::Packed => {}
            }

            sleep(self.store.config.rpc_polling_interval);
        }

        let seed = call_rpc! {
            self.rpc,
            assign_seed,
            sector_id.clone(),
        }?;

        Ok(Event::AssignSeed(seed))
    }

    fn handle_seed_assigned(&mut self) -> HandleResult {
        let token = self.limit.acquire(Stage::C1).crit()?;

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
        .abort()?;

        drop(token);
        Ok(Event::C1(out))
    }

    fn handle_c1_done(&mut self) -> HandleResult {
        let token = self.limit.acquire(Stage::C2).crit()?;

        let c1out = fetch_cloned_field! {
            self.sector.phases.c1out
        }?;

        let (prover_id, sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let out = seal_commit_phase2(c1out, prover_id, sector_id).abort()?;

        drop(token);
        Ok(Event::C2(out))
    }

    fn handle_c2_done(&mut self) -> HandleResult {
        // TODO: do the copy
        let allocated = fetch_field! {
            self.sector.base,
            allocated,
        }?;

        let sector_id = &allocated.id;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let mut wanted = vec![sealed_file];

        let sector_size = allocated.proof_type.sector_size();

        // we should be careful here, use failure as temporary
        clear_cache(sector_size.0, &cache_dir).temp()?;
        debug!(
            dir = debug_field(&cache_dir),
            "clean up unnecessary cached files"
        );

        let matches = match cache_dir.join("*").as_os_str().to_str() {
            Some(s) => glob(s).temp()?,
            None => return Err(anyhow!("invalid glob pattern under {:?}", cache_dir).crit()),
        };

        for res in matches {
            match res {
                Ok(p) => wanted.push(p),
                Err(e) => {
                    let ctx = format!("glob error for {:?}", e.path());
                    return Err(e.into_error().crit()).context(ctx);
                }
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
            let size = self.remote_store.put(target_path, source).crit()?;

            debug!(size, "persist done");

            drop(copy_enter);
        }

        Ok(Event::Persist)
    }

    fn handle_persisted(&mut self) -> HandleResult {
        let sector_id = fetch_cloned_field! {
            self.sector.base,
            allocated.id,
        }?;

        let info = ProofOnChainInfo {
            proof: fetch_cloned_field! {
                self.sector.phases.c2out,
                proof,
            }?,
        };

        let res = call_rpc! {
            self.rpc,
            submit_proof,
            sector_id,
            info,
        }?;

        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitProof),

            SubmitResult::MismatchedSubmission => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
            }

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).temp()),
        }
    }

    fn handle_proof_submitted(&mut self) -> HandleResult {
        // TODO: check proof landed on chain
        warn!("proof submitted");
        Ok(Event::Finish)
    }
}

pub struct Worker<O> {
    store: Store,
    resume_rx: Receiver<()>,
    done_rx: Receiver<()>,
    rpc: Arc<SealerRpcClient>,
    remote_store: Arc<O>,
    limit: Arc<Pool>,
}

impl<O: ObjectStore> Worker<O> {
    pub fn new(
        s: Store,
        resume_rx: Receiver<()>,
        done_rx: Receiver<()>,
        rpc: Arc<SealerRpcClient>,
        remote_store: Arc<O>,
        limit: Arc<Pool>,
    ) -> Self {
        Worker {
            store: s,
            resume_rx,
            done_rx,
            rpc,
            remote_store,
            limit,
        }
    }

    pub fn start_seal(&mut self) -> Result<()> {
        let span = info_span!("seal", loc = debug_field(&self.store.location));
        let _span = span.enter();

        let mut wait_for_resume = false;
        'SEAL_LOOP: loop {
            if wait_for_resume {
                warn!("waiting for resume signal");

                select! {
                    recv(self.resume_rx) -> resume_res => {
                        resume_res.context("resume signal channel closed unexpectedly")?;
                    },

                    recv(self.done_rx) -> _done_res => {
                        return Ok(())
                    },
                }
            }

            if self.done_rx.try_recv() != Err(TryRecvError::Empty) {
                return Ok(());
            }

            if let Err(failure) = self.seal_one() {
                error!(failure = debug_field(&failure), "sealing failed");
                match failure.0 {
                    Level::Temporary | Level::Permanent | Level::Critical => {
                        if failure.0 == Level::Temporary {
                            error!("temporary error should not be popagated to the top level");
                        };

                        wait_for_resume = true;
                        continue 'SEAL_LOOP;
                    }

                    Level::Abort => {}
                };
            }

            info!(
                duration = debug_field(self.store.config.seal_interval),
                "wait before sealing"
            );

            sleep(self.store.config.seal_interval);
        }
    }

    fn seal_one(&mut self) -> Result<(), Failure> {
        let mut ctx = Ctx::build(
            &self.store,
            self.rpc.clone(),
            self.remote_store.clone(),
            self.limit.clone(),
        )?;

        let mut event = None;
        loop {
            let span = info_span!(
                "seal-proc",
                miner = debug_field(ctx.sector.base.as_ref().map(|b| b.allocated.id.miner)),
                sector = debug_field(ctx.sector.base.as_ref().map(|b| b.allocated.id.number)),
                state = debug_field(ctx.sector.state),
                event = debug_field(&event),
            );

            let enter = span.enter();

            debug!("handling");

            match ctx.handle(event.take()) {
                Ok(Some(evt)) => {
                    event.replace(evt);
                }

                Ok(None) => {
                    ctx.finalize()?;
                    return Ok(());
                }

                Err(Failure(Level::Temporary, terr)) => {
                    if ctx.sector.retry >= ctx.store.config.max_retries {
                        return Err(terr.perm());
                    }

                    ctx.sync(|s| {
                        warn!(retry = s.retry, "temp error occurred: {:?}", terr,);

                        s.retry += 1;

                        Ok(())
                    })?;

                    info!(
                        interval = debug_field(ctx.store.config.recover_interval),
                        "wait before recovering"
                    );
                    sleep(ctx.store.config.recover_interval)
                }

                Err(f) => return Err(f),
            }

            drop(enter);
        }
    }
}
