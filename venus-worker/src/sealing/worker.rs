use std::error::Error as StdError;
use std::fs::{create_dir_all, OpenOptions};
use std::io::{self, prelude::*};
use std::path::PathBuf;
use std::sync::Arc;

use anyhow::{anyhow, Context, Error, Result};
use async_std::task::block_on;
use crossbeam_channel::{select, Receiver, TryRecvError};
use filecoin_proofs_api::seal::{
    add_piece, seal_commit_phase1, seal_commit_phase2, seal_pre_commit_phase1,
    seal_pre_commit_phase2,
};

use crate::logging::{debug, debug_field, error, info, info_span, warn};
use crate::metadb::{rocks::RocksMeta, MetaDocumentDB, MetaError, PrefixedMetaDB};
use crate::rpc::{
    self, OnChainState, PreCommitOnChainInfo, ProofOnChainInfo, SealerRpcClient, SectorID,
    SubmitResult,
};

use super::event::Event;
use super::sector::{PaddedBytesAmount, Sector, State, Trace, UnpaddedBytesAmount};
use super::store::Store;

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

macro_rules! impl_failure_error {
    ($name:ident, $ename:ident) => {
        #[derive(Debug)]
        struct $name(Error);

        impl $name {
            #[allow(dead_code)]
            fn new<E>(e: E) -> Self
            where
                E: StdError + Send + Sync + 'static,
            {
                Error::new(e).into()
            }
        }

        impl From<Error> for $name {
            fn from(val: Error) -> Self {
                $name(val)
            }
        }

        impl From<$name> for Failure {
            fn from(val: $name) -> Self {
                Failure::$ename(val)
            }
        }
    };
}

impl_failure_error! {TemporaryError, Temporary}
impl_failure_error! {UnrecoverableError, Unrecoverable}
impl_failure_error! {PermanentError, Permanent}
impl_failure_error! {CriticalError, Critical}

enum Failure {
    Temporary(TemporaryError),
    Unrecoverable(UnrecoverableError),
    Permanent(PermanentError),
    Critical(CriticalError),
}

impl std::fmt::Debug for Failure {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Failure::Temporary(e) => f.write_str(&format!("Temporary: {:?}", e.0)),
            Failure::Unrecoverable(e) => f.write_str(&format!("Unrecoverable: {:?}", e.0)),
            Failure::Permanent(e) => f.write_str(&format!("Permanent: {:?}", e.0)),
            Failure::Critical(e) => f.write_str(&format!("Critical: {:?}", e.0)),
        }
    }
}

type HandleResult = Result<Event, Failure>;

macro_rules! call_rpc {
    ($client:expr, $method:ident, $($arg:expr,)*) => {
        block_on($client.$method(
            $(
                $arg,
            )*
        )).map_err(|e| TemporaryError::from(anyhow!("rpc error: {:?}", e)))
    };
}

macro_rules! fetch_cloned_field {
    ($target:expr, $first_field:ident$(.$field:ident)*) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .cloned()
            .ok_or(PermanentError::from(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            )))
    };

    ($target:expr, $first_field:ident$(.$field:ident)*,) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .cloned()
            .ok_or(PermanentError::from(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            )))
    };

    ($target:expr) => {
        $target.as_ref().cloned().ok_or(PermanentError::from(anyhow!(
            "field {} is required",
            stringify!($target)
        )))
    };
}

macro_rules! fetch_field {
    ($target:expr, $first_field:ident$(.$field:ident)*) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .ok_or(PermanentError::from(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            )))
    };

    ($target:expr, $first_field:ident$(.$field:ident)*,) => {
        $target
            .as_ref()
            .map(|b| &b.$first_field$(.$field)*)
            .ok_or(PermanentError::from(anyhow!(
                "field {} is required",
                stringify!($first_field$(.$field)*)
            )))
    };

    ($target:expr) => {
        $target.as_ref().ok_or(PermanentError::from(anyhow!(
            "field {} is required",
            stringify!($target)
        )))
    };
}

struct Ctx<'c> {
    sector: Sector,
    _trace: Vec<Trace>,

    store: &'c Store,
    rpc: Arc<SealerRpcClient>,
    sector_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
    _trace_meta: MetaDocumentDB<PrefixedMetaDB<'c, RocksMeta>>,
}

impl<'c> Ctx<'c> {
    fn build(s: &'c Store, rpc: Arc<SealerRpcClient>) -> Result<Self, CriticalError> {
        let sector_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_META_PREFIX, &s.meta));

        let sector: Sector = sector_meta.get(SECTOR_INFO_KEY).or_else(|e| match e {
            MetaError::NotFound => {
                let empty = Default::default();
                sector_meta.set(SECTOR_INFO_KEY, &empty)?;
                Ok(empty)
            }

            MetaError::Failure(ie) => Err(ie),
        })?;

        let trace_meta = MetaDocumentDB::wrap(PrefixedMetaDB::wrap(SECTOR_TRACE_PREFIX, &s.meta));

        Ok(Ctx {
            sector,
            _trace: Vec::with_capacity(16),

            store: s,
            rpc,

            sector_meta,
            _trace_meta: trace_meta,
        })
    }

    fn sync<F: FnOnce(&mut Sector) -> Result<()>>(
        &mut self,
        modify_fn: F,
    ) -> Result<(), CriticalError> {
        modify_fn(&mut self.sector)?;
        self.sector_meta
            .set(SECTOR_INFO_KEY, &self.sector)
            .map_err(From::from)
    }

    fn finalize(self) -> Result<(), CriticalError> {
        self.store.cleanup()?;
        self.sector_meta.remove(SECTOR_INFO_KEY)?;
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

                    self.store
                        .config
                        .recover_interval
                        .map(|d| std::thread::sleep(d));
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
        create_dir_all(&cache_dir).map_err(CriticalError::new)?;

        let mut opt = OpenOptions::new();
        opt.create(true).read(true).write(true).truncate(true);

        {
            let staged_file = opt
                .open(self.staged_file(&sector.id))
                .map_err(CriticalError::new)?;
            drop(staged_file);
        }

        {
            let sealed_file = opt
                .open(self.sealed_file(&sector.id))
                .map_err(CriticalError::new)?;
            drop(sealed_file);
        }

        Ok(Event::Allocate(sector))
    }

    fn handle_allocated(&mut self) -> HandleResult {
        if !self.store.config.enable_deal {
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
            .map_err(PermanentError::new)?;

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
        .map_err(PermanentError::from)?;

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
        .map_err(PermanentError::from)?;

        Ok(Event::PC1(out))
    }

    fn handle_pc1_done(&mut self) -> HandleResult {
        let pc1out = fetch_cloned_field! {
            self.sector.phases.pc1out
        }?;

        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        let out =
            seal_pre_commit_phase2(pc1out, cache_dir, sealed_file).map_err(PermanentError::from)?;

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
                Err(PermanentError::from(anyhow!("{:?}: {:?}", res.res, res.desc)).into())
            }

            SubmitResult::Rejected => {
                Err(TemporaryError::from(anyhow!("{:?}: {:?}", res.res, res.desc)).into())
            }
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
                    return Err(
                        PermanentError::from(anyhow!("pre commit on-chain info not found")).into(),
                    )
                }
                OnChainState::Pending | OnChainState::Packed => {}
            }

            self.store
                .config
                .rpc_poll_interval
                .as_ref()
                .map(|d| std::thread::sleep(*d));
        }

        let seed = call_rpc! {
            self.rpc,
            assign_seed,
            sector_id.clone(),
        }?;

        Ok(Event::AssignSeed(seed))
    }

    fn handle_seed_assigned(&mut self) -> HandleResult {
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
        .map_err(PermanentError::from)?;

        Ok(Event::C1(out))
    }

    fn handle_c1_done(&mut self) -> HandleResult {
        let c1out = fetch_cloned_field! {
            self.sector.phases.c1out
        }?;

        let (prover_id, sector_id) = fetch_cloned_field! {
            self.sector.base,
            prove_input,
        }?;

        let out = seal_commit_phase2(c1out, prover_id, sector_id).map_err(PermanentError::from)?;

        Ok(Event::C2(out))
    }

    fn handle_c2_done(&mut self) -> HandleResult {
        // TODO: do the copy
        let sector_id = fetch_field! {
            self.sector.base,
            allocated.id,
        }?;

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        warn!(
            cache_dir = debug_field(&cache_dir),
            sealed_file = debug_field(&sealed_file),
            "we are copying"
        );

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
                Err(PermanentError::from(anyhow!("{:?}: {:?}", res.res, res.desc)).into())
            }

            SubmitResult::Rejected => {
                Err(TemporaryError::from(anyhow!("{:?}: {:?}", res.res, res.desc)).into())
            }
        }
    }

    fn handle_proof_submitted(&mut self) -> HandleResult {
        // TODO: check proof landed on chain
        warn!("proof submitted");
        Ok(Event::Finish)
    }
}

pub struct Worker {
    store: Store,
    resume_rx: Receiver<()>,
    done_rx: Receiver<()>,
    rpc: Arc<SealerRpcClient>,
}

impl Worker {
    pub fn new(
        s: Store,
        resume_rx: Receiver<()>,
        done_rx: Receiver<()>,
        rpc: Arc<SealerRpcClient>,
    ) -> Self {
        Worker {
            store: s,
            resume_rx,
            done_rx,
            rpc,
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
                match failure {
                    Failure::Temporary(_) | Failure::Unrecoverable(_) | Failure::Critical(_) => {
                        if let Failure::Temporary(_) = failure {
                            error!("temporary error should not be popagated to the top level");
                        };

                        wait_for_resume = true;
                        continue 'SEAL_LOOP;
                    }

                    Failure::Permanent(_) => {}
                };
            }

            self.store.config.seal_interval.as_ref().map(|d| {
                info!(duration = debug_field(d), "wait before sealing");
                std::thread::sleep(*d);
            });
        }
    }

    fn seal_one(&mut self) -> Result<(), Failure> {
        let mut ctx = Ctx::build(&self.store, self.rpc.clone())?;

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

                Err(Failure::Temporary(terr)) => {
                    if ctx.sector.retry >= ctx.store.config.max_retries {
                        return Err(Failure::Unrecoverable(terr.0.into()));
                    }

                    ctx.sync(|s| {
                        warn!(retry = s.retry, "temp error occurred: {:?}", terr.0,);

                        s.retry += 1;

                        Ok(())
                    })?;

                    ctx.store.config.recover_interval.as_ref().map(|d| {
                        info!(d = format!("{:?}", d).as_str(), "wait before recovering");
                        std::thread::sleep(*d);
                    });
                }

                Err(pf @ Failure::Permanent(_)) => return Err(pf),

                Err(cf @ Failure::Critical(_)) => return Err(cf),

                Err(uf @ Failure::Unrecoverable(_)) => return Err(uf),
            }

            drop(enter);
        }
    }
}
