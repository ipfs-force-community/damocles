use std::fs::{create_dir_all, OpenOptions};
use std::io::{self, prelude::*};
use std::path::PathBuf;
use std::thread::sleep;
use std::time::Duration;

use anyhow::{anyhow, Result};
use async_std::task::block_on;
use glob::glob;

use crate::logging::{debug, debug_field, debug_span, info, info_span, warn};
use crate::metadb::{rocks::RocksMeta, MetaDocumentDB, MetaError, PrefixedMetaDB};
use crate::rpc::{
    self, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo, SectorID, SubmitResult,
};
use crate::sealing::seal::{add_piece, clear_cache, seal_commit_phase1, seal_pre_commit_phase1};
use crate::watchdog::Ctx;

use super::{
    super::{
        seal::{PaddedBytesAmount, Stage, UnpaddedBytesAmount},
        store::Store,
    },
    *,
};

use super::HandleResult;

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
    store: &'c Store,

    sector_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
    _trace_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
}

impl<'c> Sealer<'c> {
    pub fn build(ctx: &'c Ctx, s: &'c Store) -> Result<Self, Failure> {
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
            store: s,

            sector_meta,
            _trace_meta: trace_meta,
        })
    }

    pub fn seal(mut self) -> Result<(), Failure> {
        let mut event = None;
        loop {
            let span = info_span!(
                "seal",
                miner = debug_field(self.sector.base.as_ref().map(|b| b.allocated.id.miner)),
                sector = debug_field(self.sector.base.as_ref().map(|b| b.allocated.id.number)),
                event = debug_field(&event),
            );

            let enter = span.enter();

            match self.handle(event.take()) {
                Ok(Some(evt)) => {
                    event.replace(evt);
                }

                Ok(None) => {
                    self.finalize()?;
                    return Ok(());
                }

                Err(Failure(Level::Temporary, terr)) => {
                    if self.sector.retry >= self.store.config.max_retries {
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

        debug!("handling");

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
            self.ctx.global.rpc,
            allocate_sector,
            rpc::AllocateSectorSpec {
                allowed_miners: None,
                allowed_proof_types: None,
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
            self.ctx.global.rpc,
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
            proof_type.into(),
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

        let out = self
            .ctx
            .global
            .pc2
            .process(pc1out, cache_dir, sealed_file)
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
            false,
        }?;

        // TODO: handle submit reset correctly
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
                self.ctx.global.rpc,
                poll_pre_commit_state,
                sector_id.clone(),
            }?;

            match state.state {
                OnChainState::Landed => break 'POLL,
                OnChainState::NotFound => {
                    return Err(anyhow!("pre commit on-chain info not found").abort())
                }

                // TODO: handle retry for this
                OnChainState::Failed => {
                    return Err(anyhow!("pre commit on-chain info failed").abort())
                }
                OnChainState::Pending | OnChainState::Packed => {}
            }

            debug!(
                state = debug_field(state.state),
                interval = debug_field(self.store.config.rpc_polling_interval),
                "waiting for next round of polling pre commit state",
            );

            sleep(self.store.config.rpc_polling_interval);
        }

        debug!("pre commit landed");

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

            sleep(delay);
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
        .abort()?;

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
            .c2
            .process(c1out, prover_id, sector_id)
            .abort()?;

        drop(token);
        Ok(Event::C2(out))
    }

    fn handle_c2_done(&mut self) -> HandleResult {
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
        clear_cache(sector_size, &cache_dir).temp()?;
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
            let size = self
                .ctx
                .global
                .remote_store
                .put(target_path, Box::new(source))
                .crit()?;

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
            }?
            .into(),
        };

        let res = call_rpc! {
            self.ctx.global.rpc,
            submit_proof,
            sector_id,
            info,
            false,
        }?;

        // TODO: submit reset correctly
        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::SubmitProof),

            SubmitResult::MismatchedSubmission => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc).abort())
            }

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc).temp()),
        }
    }

    fn handle_proof_submitted(&mut self) -> HandleResult {
        if self.store.config.ignore_proof_check {
            warn!("proof submitted, ignoring the check");
            return Ok(Event::Finish);
        }

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        'POLL: loop {
            let state = call_rpc! {
                self.ctx.global.rpc,
                poll_proof_state,
                sector_id.clone(),
            }?;

            match state.state {
                OnChainState::Landed => break 'POLL,
                OnChainState::NotFound => {
                    return Err(anyhow!("proof on-chain info not found").abort())
                }

                // TODO: handle retry for this
                OnChainState::Failed => return Err(anyhow!("proof on-chain info failed").abort()),

                OnChainState::Pending | OnChainState::Packed => {}
            }

            debug!(
                state = debug_field(state.state),
                interval = debug_field(self.store.config.rpc_polling_interval),
                "waiting for next round of polling proof state",
            );

            sleep(self.store.config.rpc_polling_interval);
        }

        Ok(Event::Finish)
    }
}
