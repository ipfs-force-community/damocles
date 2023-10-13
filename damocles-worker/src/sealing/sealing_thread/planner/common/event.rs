use std::fmt::{self, Debug};

use anyhow::{anyhow, Result};

use super::sector::{Base, Finalized, Sector, State, UnsealInput};
use super::task::Task;
use crate::rpc::sealer::{
    AllocatedSector, Deals, SectorRebuildInfo, SectorUnsealInfo, Seed, Ticket,
};
use crate::sealing::processor::{
    to_prover_id, PieceInfo, SealCommitPhase1Output, SealCommitPhase2Output,
    SealPreCommitPhase1Output, SealPreCommitPhase2Output, SectorId,
    SnapEncodeOutput,
};
use crate::{logging::trace, metadb::MaybeDirty};

pub(crate) enum Event {
    SetState(State),

    // No specified tasks available from sector_manager.
    Idle,

    // If `Planner::exec` returns `Event::Retry` it will be retried indefinitely
    // until `Planner::exec` returns another `Event` or an error occurs
    #[allow(dead_code)]
    Retry,

    Allocate(AllocatedSector),

    AcquireDeals(Option<Deals>),

    AddPiece(Vec<PieceInfo>),

    BuildTreeD,

    AssignTicket(Option<Ticket>),

    PC1(Ticket, SealPreCommitPhase1Output),

    PC2(SealPreCommitPhase2Output),

    PC2NeedSyntheticProof(SealPreCommitPhase2Output),

    SyntheticPoRep,

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
    AllocatedRebuildSector(SectorRebuildInfo),

    CheckSealed,

    SkipSnap,

    // for unseal
    AllocatedUnsealSector(SectorUnsealInfo),

    UnsealDone(u64),

    UploadPieceDone,
}

impl Debug for Event {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let name = match self {
            Self::SetState(_) => "SetState",

            Self::Idle => "Idle",

            Self::Retry => "Retry",

            Self::Allocate(_) => "Allocate",

            Self::AcquireDeals(_) => "AcquireDeals",

            Self::AddPiece(_) => "AddPiece",

            Self::BuildTreeD => "BuildTreeD",

            Self::AssignTicket(_) => "AssignTicket",

            Self::PC1(_, _) => "PC1",

            Self::PC2(_) => "PC2",

            Self::PC2NeedSyntheticProof(_) => "PC2WithSyntheticProof",

            Self::SyntheticPoRep => "SyntheticPoRep",

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

            Self::CheckSealed => "CheckSealed",

            Self::SkipSnap => "SkipSnap",

            Self::AllocatedUnsealSector(_) => "AllocatedUnsealSector",

            Self::UnsealDone(_) => "Unsealed",

            Self::UploadPieceDone => "UploadPieceDone",
        };

        f.write_str(name)
    }
}

macro_rules! replace {
    ($target:expr, $val:expr) => {
        trace!("replacing {}", stringify!($target));
        $target.replace($val);
    };
}

macro_rules! mem_replace {
    ($target:expr, $val:expr) => {
        trace!("mem_replacing {}", stringify!($target));
        let _ = std::mem::replace(&mut $target, $val);
    };
}

impl Event {
    pub fn apply(self, state: State, task: &mut Task) -> Result<()> {
        let next = if let Event::SetState(s) = self {
            s
        } else {
            state
        };

        if next == task.sector.state {
            return Err(anyhow!("state unchanged, may enter an infinite loop"));
        }

        self.apply_changes(task.sector.inner_mut());
        task.sector.update_state(next);

        Ok(())
    }

    fn apply_changes(self, s: &mut MaybeDirty<Sector>) {
        match self {
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
                if s.phases.ticket.as_ref() != Some(&ticket) {
                    replace!(s.phases.ticket, ticket);
                }
                replace!(s.phases.pc1out, out);
            }

            Self::PC2(out) => {
                replace!(s.phases.pc2out, out);
            }

            Self::PC2NeedSyntheticProof(out) => {
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

            Self::ReSubmitPC => {
                mem_replace!(s.phases.pc2_re_submit, true);
            }

            Self::SubmitProof => {
                mem_replace!(s.phases.c2_re_submit, false);
            }

            Self::ReSubmitProof => {
                mem_replace!(s.phases.c2_re_submit, true);
            }

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

            Self::AllocatedUnsealSector(info) => {
                Self::Allocate(info.sector).apply_changes(s);
                Self::AssignTicket(Some(info.ticket)).apply_changes(s);
                replace!(
                    s.phases.unseal_in,
                    UnsealInput {
                        piece_cid: info.piece_cid,
                        comm_d: info.comm_d,
                        offset: info.offset,
                        size: info.size,
                    }
                );
                replace!(
                    s.finalized,
                    Finalized {
                        public: Default::default(),
                        private: info.private_info
                    }
                );
            }

            _ => {}
        };
    }
}
