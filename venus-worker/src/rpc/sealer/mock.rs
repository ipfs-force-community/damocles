//! provides mock impl for the SealerRpc

use std::collections::HashMap;
use std::sync::{
    atomic::{AtomicU64, Ordering},
    RwLock,
};

use fil_types::ActorID;
use jsonrpc_core::Error;

use super::*;
use crate::{
    logging::{debug_field, error, warn},
    types::SealProof,
};

/// simplest mock server implementation
pub struct SimpleMockSealerRpc {
    miner: ActorID,
    sector_number: AtomicU64,
    proof_type: SealProof,
    ticket: Ticket,
    seed: Seed,

    pre_commits: RwLock<HashMap<SectorID, PreCommitOnChainInfo>>,
    proofs: RwLock<HashMap<SectorID, ProofOnChainInfo>>,
}

impl SimpleMockSealerRpc {
    /// constructs a SimpleMockSealerRpc with given miner & seal proof type
    pub fn new(miner: ActorID, proof_type: SealProof) -> Self {
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

impl Sealer for SimpleMockSealerRpc {
    fn allocate_sector(&self, spec: AllocateSectorSpec) -> Result<Option<AllocatedSector>> {
        if let Some(false) = spec
            .allowed_miners
            .as_ref()
            .map(|miners| miners.contains(&self.miner))
        {
            return Ok(None);
        }

        if let Some(false) = spec
            .allowed_proof_types
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
        sector: AllocatedSector,
        info: PreCommitOnChainInfo,
        reset: bool,
    ) -> Result<SubmitPreCommitResp> {
        let mut pre_commits = self.pre_commits.write().map_err(|e| {
            error!(err = debug_field(&e), "acquire write lock");
            Error::internal_error()
        })?;

        if !reset {
            if let Some(_exist) = pre_commits.get(&sector.id) {
                return Ok(SubmitPreCommitResp {
                    res: SubmitResult::DuplicateSubmit,
                    desc: None,
                });
            }
        }

        pre_commits.insert(sector.id, info);
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

    fn submit_persisted(&self, id: SectorID, instance: String) -> Result<()> {
        warn!(
            "sector m-{}-s-{} has been persisted into instance {}",
            id.miner, id.number, instance
        );
        return Ok(());
    }

    fn wait_seed(&self, _id: SectorID) -> Result<WaitSeedResp> {
        Ok(WaitSeedResp {
            should_wait: false,
            delay: 0,
            seed: Some(self.seed.clone()),
        })
    }

    fn submit_proof(
        &self,
        id: SectorID,
        proof: ProofOnChainInfo,
        reset: bool,
    ) -> Result<SubmitProofResp> {
        let mut proofs = self.proofs.write().map_err(|e| {
            error!(err = debug_field(&e), "acquire write lock");
            Error::internal_error()
        })?;

        if !reset {
            if let Some(_exist) = proofs.get(&id) {
                return Ok(SubmitProofResp {
                    res: SubmitResult::DuplicateSubmit,
                    desc: None,
                });
            }
        }

        proofs.insert(id, proof);
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

    fn report_state(&self, id: SectorID, req: ReportStateReq) -> Result<()> {
        warn!(miner = id.miner, num = id.number, "report state: {:?}", req);
        Ok(())
    }

    fn report_finalized(&self, id: SectorID) -> Result<()> {
        warn!(miner = id.miner, num = id.number, "report finalized");
        Ok(())
    }
}
