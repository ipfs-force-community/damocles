use std::collections::HashMap;
use std::sync::{
    atomic::{AtomicU64, Ordering},
    RwLock,
};

use fil_types::ActorID;
use filecoin_proofs_api::{
    seal::{SealCommitPhase2Output, SealPreCommitPhase2Output},
    RegisteredSealProof,
};
use jsonrpc_core::Error;

use super::*;
use crate::logging::{debug_field, error};

/// simplest mock server implementation
pub struct SimpleMockSealerRpc {
    miner: ActorID,
    sector_number: AtomicU64,
    proof_type: RegisteredSealProof,
    ticket: Ticket,
    seed: Seed,

    pre_commits: RwLock<HashMap<SectorID, SealPreCommitPhase2Output>>,
    proofs: RwLock<HashMap<SectorID, SealCommitPhase2Output>>,
}

impl SimpleMockSealerRpc {
    /// constructs a SimpleMockSealerRpc with given miner & seal proof type
    pub fn new(miner: ActorID, proof_type: RegisteredSealProof) -> Self {
        SimpleMockSealerRpc {
            miner,
            sector_number: Default::default(),
            proof_type,
            ticket: Default::default(),
            seed: Default::default(),
            pre_commits: RwLock::new(Default::default()),
            proofs: RwLock::new(Default::default()),
        }
    }
}

impl SealerRpc for SimpleMockSealerRpc {
    fn allocate_sector(&self, spec: AllocateSectorSpec) -> Result<Option<AllocatedSector>> {
        if let Some(false) = spec
            .allowed_miners
            .as_ref()
            .map(|miners| miners.contains(&self.miner))
        {
            return Ok(None);
        }

        if let Some(false) = spec
            .allowed_proot_types
            .as_ref()
            .map(|types| types.contains(&self.proof_type))
        {
            return Ok(None);
        }

        let next = self.sector_number.fetch_add(1, Ordering::SeqCst);

        Ok(Some(AllocatedSector {
            id: SectorID {
                miner: self.miner,
                number: next,
            },
            proof_type: self.proof_type,
        }))
    }

    fn acquire_deals(&self, _id: SectorID, _spec: AcquireDealsSpec) -> Result<Option<Deals>> {
        Ok(None)
    }

    fn assign_ticket(&self, _id: SectorID) -> Result<Ticket> {
        Ok(self.ticket.clone())
    }

    fn submit_pre_commit(
        &self,
        id: SectorID,
        out: SealPreCommitPhase2Output,
    ) -> Result<SubmitPreCommitResp> {
        let mut pre_commits = self.pre_commits.write().map_err(|e| {
            error!(err = debug_field(&e), "acquire write lock");
            Error::internal_error()
        })?;

        if let Some(exist) = pre_commits.get(&id) {
            if exist.registered_proof == out.registered_proof
                && exist.comm_r == out.comm_r
                && exist.comm_d == out.comm_d
            {
                return Ok(SubmitPreCommitResp {
                    res: SubmitResult::DuplicateSubmit,
                    desc: None,
                });
            }

            return Ok(SubmitPreCommitResp {
                res: SubmitResult::MismatchedSubmission,
                desc: None,
            });
        }

        pre_commits.insert(id, out);
        Ok(SubmitPreCommitResp {
            res: SubmitResult::Accepted,
            desc: None,
        })
    }

    fn poll_pre_commit_state(&self, id: SectorID) -> Result<PollPreCommitStateResp> {
        let pre_commits = self.pre_commits.read().map_err(|e| {
            error!(err = debug_field(&e), "acquire read lock");
            Error::internal_error()
        })?;

        match pre_commits.get(&id) {
            Some(_) => Ok(PollPreCommitStateResp {
                state: OnChainState::Landed,
                desc: None,
            }),

            None => Ok(PollPreCommitStateResp {
                state: OnChainState::NotFound,
                desc: None,
            }),
        }
    }

    fn assign_seed(&self, _id: SectorID) -> Result<Seed> {
        Ok(self.seed.clone())
    }

    fn submit_proof(&self, id: SectorID, out: SealCommitPhase2Output) -> Result<SubmitProofResp> {
        let mut proofs = self.proofs.write().map_err(|e| {
            error!(err = debug_field(&e), "acquire write lock");
            Error::internal_error()
        })?;

        if let Some(exist) = proofs.get(&id) {
            if exist.proof == out.proof {
                return Ok(SubmitProofResp {
                    res: SubmitResult::DuplicateSubmit,
                    desc: None,
                });
            }

            return Ok(SubmitProofResp {
                res: SubmitResult::MismatchedSubmission,
                desc: None,
            });
        }

        proofs.insert(id.clone(), out);
        Ok(SubmitProofResp {
            res: SubmitResult::Accepted,
            desc: None,
        })
    }

    fn poll_proof_state(&self, id: SectorID) -> Result<PollProofStateResp> {
        let proofs = self.proofs.read().map_err(|e| {
            error!(err = debug_field(&e), "acquire read lock");
            Error::internal_error()
        })?;

        match proofs.get(&id) {
            Some(_) => Ok(PollProofStateResp {
                state: OnChainState::Landed,
                desc: None,
            }),

            None => Ok(PollProofStateResp {
                state: OnChainState::NotFound,
                desc: None,
            }),
        }
    }
}
