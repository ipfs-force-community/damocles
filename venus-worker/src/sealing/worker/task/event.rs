use std::fmt::{self, Debug};

use anyhow::{anyhow, Result};

use super::{
    sector::{Base, Finalized, Sector, State},
    Planner,
};
use crate::logging::trace;
use crate::rpc::sealer::{AllocatedSector, Deals, SectorRebuildInfo, Seed, Ticket};
use crate::sealing::processor::{
    to_prover_id, PieceInfo, SealCommitPhase1Output, SealCommitPhase2Output, SealPreCommitPhase1Output, SealPreCommitPhase2Output,
    SectorId, SnapEncodeOutput,
};

pub enum Event {
    SetState(State),

    // No specified tasks available from sector_manager.
    Idle,

    Retry,

    Allocate(AllocatedSector),

    AcquireDeals(Option<Deals>),

    AddPiece(Vec<PieceInfo>),

    BuildTreeD,

    AssignTicket(Option<Ticket>),

    PC1(Ticket, SealPreCommitPhase1Output),

    PC2(SealPreCommitPhase2Output),

    SubmitPC,

    CheckPC,

    ReSubmitPC,

    Persist(String),

    SubmitPersistance,

    AssignSeed(Seed),

    C1(SealCommitPhase1Output),

    C2(SealCommitPhase2Output),

    SubmitProof,

    ReSubmitProof,

    Finish,

    // for snap up
    AllocatedSnapUpSector(AllocatedSector, Deals, Finalized),

    SnapEncode(SnapEncodeOutput),

    SnapProve(Vec<u8>),

    RePersist,

    // for rebuild
    // this allowance should be removed after planner has been implmented
    #[allow(dead_code)]
    AllocatedRebuildSector(SectorRebuildInfo),
}

impl Debug for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let name = match self {
            Self::SetState(_) => "SetState",

            Self::Idle => "idle",

            Self::Retry => "Retry",

            Self::Allocate(_) => "Allocate",

            Self::AcquireDeals(_) => "AcquireDeals",

            Self::AddPiece(_) => "AddPiece",

            Self::BuildTreeD => "BuildTreeD",

            Self::AssignTicket(_) => "AssignTicket",

            Self::PC1(_, _) => "PC1",

            Self::PC2(_) => "PC2",

            Self::SubmitPC => "SubmitPC",

            Self::CheckPC => "CheckPC",

            Self::ReSubmitPC => "ReSubmitPC",

            Self::Persist(_) => "Persist",

            Self::SubmitPersistance => "SubmitPersistance",

            Self::AssignSeed(_) => "AssignSeed",

            Self::C1(_) => "C1",

            Self::C2(_) => "C2",

            Self::SubmitProof => "SubmitProof",

            Self::ReSubmitProof => "ReSubmitProof",

            Self::Finish => "Finish",

            // for snap up
            Self::AllocatedSnapUpSector(_, _, _) => "AllocatedSnapUpSector",

            Self::SnapEncode(_) => "SnapEncode",

            Self::SnapProve(_) => "SnapProve",

            Self::RePersist => "RePersist",

            // for rebuild
            Self::AllocatedRebuildSector(_) => "AllocatedRebuildSector",
        };

        f.write_str(name)
    }
}

macro_rules! replace {
    ($target:expr, $val:expr) => {
        let prev = $target.replace($val);
        if let Some(st) = $target.as_ref() {
            trace!("{:?} => {:?}", prev, st);
        }
    };
}

macro_rules! mem_replace {
    ($target:expr, $val:expr) => {
        let prev = std::mem::replace(&mut $target, $val);
        trace!("{:?} => {:?}", prev, $target);
    };
}

impl Event {
    pub fn apply<P: Planner>(self, p: &P, s: &mut Sector) -> Result<()> {
        let next = if let Event::SetState(s) = self {
            s
        } else {
            p.plan(&self, &s.state)?
        };

        if next == s.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        self.apply_changes(s);
        s.update_state(next);

        Ok(())
    }

    fn apply_changes(self, s: &mut Sector) {
        match self {
            Self::SetState(_) => {}

            Self::Idle => {}

            Self::Retry => {}

            Self::Allocate(sector) => {
                let prover_id = to_prover_id(sector.id.miner);
                let sector_id = SectorId::from(sector.id.number);

                let base = Base {
                    allocated: sector,
                    prove_input: (prover_id, sector_id),
                };

                replace!(s.base, base);
            }

            Self::AcquireDeals(deals) => {
                mem_replace!(s.deals, deals);
            }

            Self::AddPiece(pieces) => {
                replace!(s.phases.pieces, pieces);
            }

            Self::BuildTreeD => {}

            Self::AssignTicket(ticket) => {
                mem_replace!(s.phases.ticket, ticket);
            }

            Self::PC1(ticket, out) => {
                replace!(s.phases.ticket, ticket);
                replace!(s.phases.pc1out, out);
            }

            Self::PC2(out) => {
                replace!(s.phases.pc2out, out);
            }

            Self::Persist(instance) => {
                replace!(s.phases.persist_instance, instance);
            }

            Self::AssignSeed(seed) => {
                replace!(s.phases.seed, seed);
            }

            Self::C1(out) => {
                replace!(s.phases.c1out, out);
            }

            Self::C2(out) => {
                replace!(s.phases.c2out, out);
            }

            Self::SubmitPC => {
                mem_replace!(s.phases.pc2_re_submit, false);
            }

            Self::CheckPC => {}

            Self::ReSubmitPC => {
                mem_replace!(s.phases.pc2_re_submit, true);
            }

            Self::SubmitProof => {
                mem_replace!(s.phases.c2_re_submit, false);
            }

            Self::ReSubmitProof => {
                mem_replace!(s.phases.c2_re_submit, true);
            }

            Self::SubmitPersistance => {}

            Self::Finish => {}

            // for snap up
            Self::AllocatedSnapUpSector(sector, deals, finalized) => {
                Self::Allocate(sector).apply_changes(s);
                replace!(s.deals, deals);
                replace!(s.finalized, finalized);
            }

            Self::SnapEncode(out) => {
                replace!(s.phases.snap_encode_out, out);
            }

            Self::SnapProve(out) => {
                replace!(s.phases.snap_prov_out, out);
            }

            Self::RePersist => {}

            // for rebuild
            Self::AllocatedRebuildSector(rebuild) => {
                Self::Allocate(rebuild.sector).apply_changes(s);
                Self::AcquireDeals(rebuild.pieces).apply_changes(s);
                Self::AssignTicket(Some(rebuild.ticket)).apply_changes(s);
                mem_replace!(
                    s.finalized,
                    rebuild.upgrade_public.map(|p| {
                        Finalized {
                            public: p,
                            private: Default::default(),
                        }
                    })
                );
            }
        };
    }
}
