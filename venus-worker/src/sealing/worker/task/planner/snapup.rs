use std::collections::HashMap;
use std::os::unix::fs::symlink;

use anyhow::{anyhow, Context, Result};
use vc_processors::builtin::tasks::{STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE};

use super::{
    super::{call_rpc, cloned_required, field_required, Finalized},
    common, plan, Event, ExecResult, Planner, State, Task,
};
use crate::logging::{debug, warn};
use crate::rpc::sealer::{AcquireDealsSpec, AllocateSectorSpec, AllocateSnapUpSpec, SnapUpOnChainInfo, SubmitResult};
use crate::sealing::failure::*;
use crate::sealing::processor::{
    snap_generate_partition_proofs, snap_verify_sector_update_proof, tree_d_path_in_dir, SnapEncodeInput, SnapProveInput, TransferItem,
    TransferInput, TransferOption, TransferRoute, TransferStoreInfo,
};

pub struct SnapUpPlanner;

impl Planner for SnapUpPlanner {
    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                Event::AllocatedSnapUpSector(_, _, _) => State::Allocated,
            },

            State::Allocated => {
                Event::AddPiece(_) => State::PieceAdded,
            },

            State::PieceAdded => {
                Event::BuildTreeD => State::TreeDBuilt,
            },

            State::TreeDBuilt => {
                Event::SnapEncode(_) => State::SnapEncoded,
            },

            State::SnapEncoded => {
                Event::SnapProve(_) => State::SnapProved,
            },

            State::SnapProved => {
                Event::Persist(_) => State::Persisted,
            },

            State::Persisted => {
                Event::RePersist => State::SnapProved,
                Event::Finish => State::Finished,
            },
        };

        Ok(next)
    }

    fn exec<'t>(&self, task: &'t mut Task<'_>) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let inner = SnapUp { task };
        match state {
            State::Empty => inner.empty(),

            State::Allocated => inner.add_piece(),

            State::PieceAdded => inner.build_tree_d(),

            State::TreeDBuilt => inner.snap_encode(),

            State::SnapEncoded => inner.snap_prove(),

            State::SnapProved => inner.persist(),

            State::Persisted => inner.submit(),

            State::Finished => return Ok(None),

            State::Aborted => {
                warn!("sector aborted");
                return Ok(None);
            }

            other => return Err(anyhow!("unexpected state {:?} in snapup planner", other).abort()),
        }
        .map(From::from)
    }
}

struct SnapUp<'c, 't> {
    task: &'t mut Task<'c>,
}

impl<'c, 't> SnapUp<'c, 't> {
    fn empty(&self) -> ExecResult {
        let maybe_res = call_rpc! {
            self.task.ctx.global.rpc,
            allocate_snapup_sector,
            AllocateSnapUpSpec {
                sector: AllocateSectorSpec {
                    allowed_miners: self.task.store.allowed_miners.as_ref().cloned(),
                    allowed_proof_types: self.task.store.allowed_proof_types.as_ref().cloned(),
                },
                deals: AcquireDealsSpec {
                    max_deals: self.task.store.config.max_deals,
                    min_used_space: self.task.store.config.min_deal_space.map(|b| b.get_bytes() as usize),
                },
            },
        };

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Retry);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Retry),
        };

        if allocated.pieces.is_empty() {
            return Err(anyhow!("deals required, got empty").abort());
        }

        if allocated.private.access_instance.is_empty() {
            return Err(anyhow!("access instance required, got empty").abort());
        }

        Ok(Event::AllocatedSnapUpSector(
            allocated.sector,
            allocated.pieces,
            Finalized {
                public: allocated.public,
                private: allocated.private,
            },
        ))
    }

    fn add_piece(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        let mut staged_file = self.task.staged_file(sector_id).init_file().perm()?;
        field_required!(deals, self.task.sector.deals.as_ref());

        let pieces = common::add_pieces(self.task, proof_type.into(), &mut staged_file, deals)?;

        Ok(Event::AddPiece(pieces))
    }

    fn build_tree_d(&self) -> ExecResult {
        common::build_tree_d(self.task, false)?;
        Ok(Event::BuildTreeD)
    }

    fn snap_encode(&self) -> ExecResult {
        let _token = self.task.ctx.global.limit.acquire(STAGE_NAME_SNAP_ENCODE).crit()?;

        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;
        field_required!(
            access_instance,
            self.task.sector.finalized.as_ref().map(|f| &f.private.access_instance)
        );

        cloned_required!(piece_infos, self.task.sector.phases.pieces);

        debug!("find access store named {}", access_instance);
        let access_store = self
            .task
            .ctx
            .global
            .attached
            .get(access_instance)
            .with_context(|| format!("get access store instance named {}", access_instance))
            .perm()?;

        debug!("get basic info for access store named {}", access_instance);
        let access_store_basic_info = call_rpc! {
            self.task.ctx.global.rpc,
            store_basic_info,
            access_instance.clone(),
        }?
        .with_context(|| format!("get basic info for store named {}", access_instance))
        .perm()?;

        // sealed file & persisted cache files should be accessed inside persist store
        let sealed_file = self.task.sealed_file(sector_id);
        sealed_file.prepare().perm()?;
        let sealed_rel = sealed_file.rel();

        let cache_dir = self.task.cache_dir(sector_id);
        let cache_rel = cache_dir.rel();

        let transfer_routes = vec![
            TransferRoute {
                src: TransferItem {
                    store_name: Some(access_instance.clone()),
                    uri: access_store
                        .uri(sealed_rel)
                        .with_context(|| format!("get uri for sealed file {:?} in {}", sealed_rel, access_instance))
                        .perm()?,
                },
                dest: TransferItem {
                    store_name: None,
                    uri: sealed_file.full().clone(),
                },
                opt: Some(TransferOption {
                    is_dir: false,
                    allow_link: true,
                }),
            },
            TransferRoute {
                src: TransferItem {
                    store_name: Some(access_instance.clone()),
                    uri: access_store
                        .uri(cache_rel)
                        .with_context(|| format!("get uri for cache dir {:?} in {}", cache_rel, access_instance))
                        .perm()?,
                },
                dest: TransferItem {
                    store_name: None,
                    uri: cache_dir.full().clone(),
                },
                opt: Some(TransferOption {
                    is_dir: true,
                    allow_link: true,
                }),
            },
        ];

        let transfer = TransferInput {
            stores: HashMap::from_iter([(
                access_instance.clone(),
                TransferStoreInfo {
                    name: access_instance.clone(),
                    meta: access_store_basic_info.meta,
                },
            )]),
            routes: transfer_routes,
        };

        self.task
            .ctx
            .global
            .processors
            .transfer
            .process(transfer)
            .context("link snapup sector files")
            .perm()?;

        // init update file
        let update_file = self.task.update_file(sector_id);
        debug!(path=?update_file.full(),  "trying to init update file");
        {
            let file = update_file.init_file().perm()?;
            file.set_len(proof_type.sector_size()).context("fallocate for update file").perm()?;
        }

        let update_cache_dir = self.task.update_cache_dir(sector_id);
        debug!(path=?update_cache_dir.full(),  "trying to init update cache dir");
        update_cache_dir.prepare().context("prepare update cache dir").perm()?;

        // tree d
        debug!("trying to prepare tree_d");
        let prepared_dir = self.task.prepared_dir(sector_id);
        symlink(
            tree_d_path_in_dir(prepared_dir.as_ref()),
            tree_d_path_in_dir(update_cache_dir.as_ref()),
        )
        .context("link prepared tree_d")
        .crit()?;

        // staged file should be already exists, do nothing
        let staged_file = self.task.staged_file(sector_id);

        let snap_encode_out = self
            .task
            .ctx
            .global
            .processors
            .snap_encode
            .process(SnapEncodeInput {
                registered_proof: proof_type.into(),
                new_replica_path: update_file.into(),
                new_cache_path: update_cache_dir.into(),
                sector_path: sealed_file.into(),
                sector_cache_path: cache_dir.into(),
                staged_data_path: staged_file.into(),
                piece_infos,
            })
            .perm()?;

        Ok(Event::SnapEncode(snap_encode_out))
    }

    fn snap_prove(&self) -> ExecResult {
        let _token = self.task.ctx.global.limit.acquire(STAGE_NAME_SNAP_PROVE).crit()?;

        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;
        field_required!(encode_out, self.task.sector.phases.snap_encode_out.as_ref());
        field_required!(comm_r_old, self.task.sector.finalized.as_ref().map(|f| f.public.comm_r));

        let sealed_file = self.task.sealed_file(sector_id);
        let cached_dir = self.task.cache_dir(sector_id);
        let update_file = self.task.update_file(sector_id);
        let update_cache_dir = self.task.update_cache_dir(sector_id);

        let vannilla_proofs = snap_generate_partition_proofs(
            (*proof_type).into(),
            comm_r_old,
            encode_out.comm_r_new,
            encode_out.comm_d_new,
            sealed_file.into(),
            cached_dir.into(),
            update_file.into(),
            update_cache_dir.into(),
        )
        .perm()?;

        let proof = self
            .task
            .ctx
            .global
            .processors
            .snap_prove
            .process(SnapProveInput {
                registered_proof: (*proof_type).into(),
                vannilla_proofs: vannilla_proofs.into_iter().map(|b| b.0).collect(),
                comm_r_old,
                comm_r_new: encode_out.comm_r_new,
                comm_d_new: encode_out.comm_d_new,
            })
            .perm()?;

        let verified =
            snap_verify_sector_update_proof(proof_type.into(), &proof, comm_r_old, encode_out.comm_r_new, encode_out.comm_d_new).perm()?;

        if !verified {
            return Err(anyhow!("generated an invalid update proof").perm());
        }

        Ok(Event::SnapProve(proof))
    }

    fn persist(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        let update_cache_dir = self.task.update_cache_dir(sector_id);
        let update_file = self.task.update_file(sector_id);

        let ins_name = common::persist_sector_files(self.task, update_cache_dir, update_file)?;

        Ok(Event::Persist(ins_name))
    }

    fn submit(&self) -> ExecResult {
        let sector_id = self.task.sector_id()?;
        field_required!(proof, self.task.sector.phases.snap_prov_out.as_ref());
        field_required!(deals, self.task.sector.deals.as_ref());
        field_required!(encode_out, self.task.sector.phases.snap_encode_out.as_ref());
        cloned_required!(instance, self.task.sector.phases.persist_instance);
        let piece_cids = deals.iter().map(|d| d.piece.cid.clone()).collect();

        let res = call_rpc! {
            self.task.ctx.global.rpc,
            submit_snapup_proof,
            sector_id.clone(),
            SnapUpOnChainInfo {
                comm_r: encode_out.comm_r_new,
                comm_d: encode_out.comm_d_new,
                access_instance: instance,
                pieces: piece_cids,
                proof: proof.into(),
            },
        }?;

        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => Ok(Event::Finish),

            SubmitResult::MismatchedSubmission => Err(anyhow!(
                "submission for {} is not matched with a previous one: {:?}",
                self.task.sector_path(sector_id),
                res.desc
            )
            .abort()),

            SubmitResult::Rejected => Err(anyhow!("{:?}: {:?}", res.res, res.desc)).abort(),

            SubmitResult::FilesMissed => Ok(Event::RePersist),
        }
    }
}
