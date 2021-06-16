use std::sync::Arc;

use anyhow::{anyhow, Context, Error, Result};
use async_std::task::block_on;
use crossbeam_channel::{select, Receiver, TryRecvError};

use crate::logging::{debug, debug_field, error, info, info_span, warn};
use crate::metadb::{rocks::RocksMeta, MetaDocumentDB, MetaError, PrefixedMetaDB};
use crate::rpc::{self, SealerRpcClient, SectorID};

use super::event::Event;
use super::sector::{Sector, State, Trace};
use super::store::Store;

const SECTOR_INFO_KEY: &str = "info";
const SECTOR_META_PREFIX: &str = "meta";
const SECTOR_TRACE_PREFIX: &str = "trace";

macro_rules! impl_failure_error {
    ($name:ident, $ename:ident) => {
        #[derive(Debug)]
        struct $name(Error);

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

    fn sector_id(&self) -> Result<SectorID, PermanentError> {
        self.sector
            .base
            .as_ref()
            .map(|b| &b.id)
            .cloned()
            .ok_or(PermanentError::from(anyhow!("sector id is required")))
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

            State::DealsAcquired => self.handle_deal_acquired(),

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
        let allocated = call_rpc! {
            self.rpc,
            allocate_sector,
            rpc::AllocateSectorSpec {
                allowed_miners: None,
                allowed_proot_types: None,
            },
        }?;

        match allocated {
            Some(a) => Ok(Event::Allocate(a)),
            None => Ok(Event::Retry),
        }
    }

    fn handle_allocated(&mut self) -> HandleResult {
        if !self.store.config.enable_deal {
            return Ok(Event::AcquireDeals(None));
        }

        let deals = call_rpc! {
            self.rpc,
            acquire_deals,
            self.sector_id()?,
            rpc::AcquireDealsSpec {
                max_deals: None,
            },
        }?;

        Ok(Event::AcquireDeals(deals))
    }

    fn handle_deal_acquired(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_piece_added(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_ticket_assigned(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_pc1_done(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_pc2_done(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_pc_submitted(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_seed_assigned(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_c1_done(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_c2_done(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_persisted(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_proof_submitted(&mut self) -> HandleResult {
        unimplemented!();
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
                miner = debug_field(ctx.sector.base.as_ref().map(|b| b.id.miner)),
                sector = debug_field(ctx.sector.base.as_ref().map(|b| b.id.number)),
                state = debug_field(ctx.sector.state),
                event = debug_field(&event),
            );

            let enter = span.enter();

            match ctx.handle(event.take()) {
                Ok(Some(evt)) => {
                    event.replace(evt);
                }

                Ok(None) => return Ok(()),

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
