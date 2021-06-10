use anyhow::{anyhow, Context, Error, Result};
use serde_json::to_vec;

use crate::metadb::MetaDB;

use event::Event;
use sector::State;

mod event;
mod sector;
mod store;

const sector_info_key: &str = "sector";

struct Ctx<'m, MS: MetaDB, MT: MetaDB> {
    sector: sector::Sector,
    trace: Vec<sector::Trace>,

    sector_meta: &'m MS,
    trace_meta: &'m MT,
}

impl<'m, MS, MT> Ctx<'m, MS, MT>
where
    MS: MetaDB,
    MT: MetaDB,
{
    fn sync(&mut self) -> Result<()> {
        let data = to_vec(&self.sector).context("json marshal")?;
        self.sector_meta
            .set(sector_info_key, data)
            .context("set sector info")
    }
}

struct TemporaryError(Error);

impl From<Error> for TemporaryError {
    fn from(val: Error) -> Self {
        TemporaryError(val)
    }
}

struct PermanentError(Error);

impl From<Error> for PermanentError {
    fn from(val: Error) -> Self {
        PermanentError(val)
    }
}

enum Failure {
    Temporary(TemporaryError),
    Permanent(PermanentError),
}

impl From<TemporaryError> for Failure {
    fn from(val: TemporaryError) -> Self {
        Failure::Temporary(val)
    }
}

impl From<PermanentError> for Failure {
    fn from(val: PermanentError) -> Self {
        Failure::Permanent(val)
    }
}

type HandleResult = Result<Event, Failure>;

struct Worker<'w, DB, MS, MT>
where
    DB: MetaDB,
    MS: MetaDB,
    MT: MetaDB,
{
    ctx: Ctx<'w, MS, MT>,
    store: store::Store<DB>,
}

impl<'w, DB, MS, MT> Worker<'w, DB, MS, MT>
where
    DB: MetaDB,
    MS: MetaDB,
    MT: MetaDB,
{
    fn handle(&mut self, evt: Option<Event>) -> Result<(), Failure> {
        if let Some(evt) = evt {
            evt.apply(&mut self.ctx.sector)
                .map_err(|e| PermanentError(e))?;
            self.ctx.sync().map_err(|e| PermanentError(e))?;
        };

        match self.ctx.sector.state {
            State::Empty => self.handle_empty().and_then(|evt| self.handle(Some(evt))),

            State::Allocated => unreachable!(),

            State::DealsAcquired => unreachable!(),

            State::PieceAdded => unreachable!(),

            State::TicketAssigned => unreachable!(),

            State::PC1Done => unreachable!(),

            State::PC2Done => unreachable!(),

            State::PCSubmitted => unreachable!(),

            State::SeedAssigned => unreachable!(),

            State::C1Done => unreachable!(),

            State::C2Done => unreachable!(),

            State::Persisted => unreachable!(),

            State::ProofSubmitted => unreachable!(),

            State::Finished => return Ok(()),
        }
    }

    fn handle_empty(&mut self) -> HandleResult {
        unimplemented!();
    }

    fn handle_allocated(&mut self) -> HandleResult {
        unimplemented!();
    }
}
