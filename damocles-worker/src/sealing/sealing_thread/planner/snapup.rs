use std::collections::HashMap;

use anyhow::{anyhow, Context, Result};

use super::{
    super::{call_rpc, cloned_required, field_required},
    common::{
        self,
        event::Event,
        sector::{Finalized, State},
        task::Task,
    },
    plan, PlannerTrait, PLANNER_NAME_SNAPUP,
};
use crate::logging::{debug, warn};
use crate::rpc::sealer::{
    AcquireDealsSpec, AllocateSectorSpec, AllocateSnapUpSpec,
    SnapUpOnChainInfo, SubmitResult,
};
use crate::sealing::failure::*;
use crate::sealing::processor::{
    cached_filenames_for_sector, TransferInput, TransferItem, TransferOption,
    TransferRoute, TransferStoreInfo,
};

#[derive(Default)]
pub(crate) struct SnapUpPlanner;

impl PlannerTrait for SnapUpPlanner {
    type Job = Task;
    type State = State;
    type Event = Event;

    fn name(&self) -> &str {
        PLANNER_NAME_SNAPUP
    }

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

    fn exec(&self, task: &mut Task) -> Result<Option<Event>, Failure> {
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
                return Err(TaskAborted.into());
            }

            other => {
                return Err(anyhow!(
                    "unexpected state {:?} in snapup planner",
                    other
                )
                .abort())
            }
        }
        .map(Some)
    }

    fn apply(&self, event: Event, state: State, task: &mut Task) -> Result<()> {
        event.apply(state, task)
    }
}

struct SnapUp<'t> {
    task: &'t mut Task,
}

impl<'t> SnapUp<'t> {
    fn empty(&self) -> Result<Event, Failure> {
        let maybe_res = call_rpc! {
            self.task.rpc() => allocate_snapup_sector(AllocateSnapUpSpec {
                sector: AllocateSectorSpec {
                    allowed_miners: Some(self.task.sealing_ctrl.config().allowed_miners.clone()),
                    allowed_proof_types: Some(self.task.sealing_ctrl.config().allowed_proof_types.clone()),
                },
                deals: AcquireDealsSpec {
                    max_deals: self.task.sealing_ctrl.config().max_deals,
                    min_used_space: self.task.sealing_ctrl.config().min_deal_space.map(|b| b.get_bytes() as usize),
                },
            },
        )};

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!("sectors are not allocated yet, so we can retry even though we got the err {:?}", e);
                return Ok(Event::Idle);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
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

    fn add_piece(&self) -> Result<Event, Failure> {
        field_required!(deals, self.task.sector.deals.as_ref());

        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;
        let pieces = common::add_pieces(
            &self.task.sealing_ctrl,
            *proof_type,
            self.task.staged_file(sector_id),
            deals,
        )?;

        Ok(Event::AddPiece(pieces))
    }

    fn build_tree_d(&self) -> Result<Event, Failure> {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        common::build_tree_d(
            &self.task.sealing_ctrl,
            *proof_type,
            self.task.prepared_dir(sector_id),
            self.task.staged_file(sector_id),
            false,
            self.task.is_cc(),
        )?;
        Ok(Event::BuildTreeD)
    }

    fn snap_encode(&self) -> Result<Event, Failure> {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;
        field_required!(
            access_instance,
            self.task
                .sector
                .finalized
                .as_ref()
                .map(|f| &f.private.access_instance)
        );

        debug!("find access store named {}", access_instance);
        let access_store = self
            .task
            .sealing_ctrl
            .ctx()
            .global
            .attached
            .get(access_instance)
            .with_context(|| {
                format!("get access store instance named {}", access_instance)
            })
            .perm()?;

        debug!("get basic info for access store named {}", access_instance);
        let access_store_basic_info = call_rpc! {
            self.task.rpc() => store_basic_info(access_instance.clone(),)
        }?
        .with_context(|| {
            format!("get basic info for store named {}", access_instance)
        })
        .perm()?;

        // sealed file & persisted cache files should be accessed inside persist store
        let sealed_file = self.task.sealed_file(sector_id);
        sealed_file.prepare().perm()?;
        let sealed_rel = sealed_file.rel();

        let cache_dir = self.task.cache_dir(sector_id);

        let cached_file_routes =
            cached_filenames_for_sector(proof_type.into(), true)
                .into_iter()
                .map(|fname| {
                    let cached_file = cache_dir.join(fname);
                    let cached_rel = cached_file.rel();

                    Ok(TransferRoute {
                        src: TransferItem {
                            store_name: Some(access_instance.clone()),
                            uri: access_store
                                .uri(cached_rel)
                                .with_context(|| {
                                    format!(
                                        "get uri for cache dir {:?} in {}",
                                        cached_rel, access_instance
                                    )
                                })
                                .perm()?,
                        },
                        dest: TransferItem {
                            store_name: None,
                            uri: cached_file.full().clone(),
                        },
                        opt: Some(TransferOption {
                            is_dir: false,
                            allow_link: true,
                        }),
                    })
                })
                .collect::<Result<Vec<_>, Failure>>()?;

        let mut transfer_routes = vec![TransferRoute {
            src: TransferItem {
                store_name: Some(access_instance.clone()),
                uri: access_store
                    .uri(sealed_rel)
                    .with_context(|| {
                        format!(
                            "get uri for sealed file {:?} in {}",
                            sealed_rel, access_instance
                        )
                    })
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
        }];

        transfer_routes.extend(cached_file_routes);

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
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .transfer
            .process(self.task.sealing_ctrl.ctrl_ctx(), transfer)
            .context("link snapup sector files")
            .perm()?;

        common::snap_encode(self.task, sector_id, proof_type)
            .map(Event::SnapEncode)
    }

    fn snap_prove(&self) -> Result<Event, Failure> {
        common::snap_prove(self.task).map(Event::SnapProve)
    }

    fn persist(&self) -> Result<Event, Failure> {
        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;
        let update_cache_dir = self.task.update_cache_dir(sector_id);
        let update_file = self.task.update_file(sector_id);

        let ins_name = common::persist_sector_files(
            &self.task.sealing_ctrl,
            *proof_type,
            sector_id,
            update_cache_dir,
            update_file,
            true,
        )?;

        Ok(Event::Persist(ins_name))
    }

    fn submit(&self) -> Result<Event, Failure> {
        let sector_id = self.task.sector_id()?;
        field_required!(proof, self.task.sector.phases.snap_prov_out.as_ref());
        field_required!(deals, self.task.sector.deals.as_ref());
        field_required!(
            encode_out,
            self.task.sector.phases.snap_encode_out.as_ref()
        );
        cloned_required!(instance, self.task.sector.phases.persist_instance);
        let piece_cids = deals.iter().map(|d| d.piece.cid.clone()).collect();

        let res = call_rpc! {
            self.task.rpc()=>submit_snapup_proof(
                sector_id.clone(),
                SnapUpOnChainInfo {
                    comm_r: encode_out.comm_r_new,
                    comm_d: encode_out.comm_d_new,
                    access_instance: instance,
                    pieces: piece_cids,
                    proof: proof.into(),
                },
            )
        }?;

        match res.res {
            SubmitResult::Accepted | SubmitResult::DuplicateSubmit => {
                Ok(Event::Finish)
            }

            SubmitResult::MismatchedSubmission => Err(anyhow!(
                "submission for {} is not matched with a previous one: {:?}",
                self.task.sector_path(sector_id),
                res.desc
            )
            .abort()),

            SubmitResult::Rejected => {
                Err(anyhow!("{:?}: {:?}", res.res, res.desc)).abort()
            }

            SubmitResult::FilesMissed => Ok(Event::RePersist),
        }
    }
}
