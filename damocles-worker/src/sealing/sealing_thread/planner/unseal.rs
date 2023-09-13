use super::{
    super::{call_rpc, field_required},
    common::{event::Event, sector::State, task::Task},
    plan, PlannerTrait, PLANNER_NAME_UNSEAL,
};
use crate::sealing::failure::*;
use crate::{filestore::Resource, logging::warn};
use crate::{
    infra::filestore::FileStoreExt,
    rpc::sealer::{AllocateSectorSpec, PathType},
};
use anyhow::{anyhow, Context, Result};
use forest_cid::Cid;
use std::{
    collections::HashMap,
    fs::{self, remove_file, File},
    path::{Path, PathBuf},
    vec,
};
use tracing::{debug, info};
use url::Url;
use vc_processors::{
    builtin::tasks::{
        PieceFile, Unseal as UnsealInput, STAGE_NAME_TRANSFER,
        STAGE_NAME_UNSEAL,
    },
    fil_proofs::{UnpaddedByteIndex, UnpaddedBytesAmount},
};

use crate::sealing::processor::{
    TransferInput, TransferItem, TransferRoute, TransferStoreInfo,
};

#[derive(Default)]
pub(crate) struct UnsealPlanner;

impl PlannerTrait for UnsealPlanner {
    type Job = Task;
    type State = State;
    type Event = Event;

    fn name(&self) -> &str {
        PLANNER_NAME_UNSEAL
    }

    fn plan(&self, evt: &Event, st: &State) -> Result<State> {
        let next = plan! {
            evt,
            st,

            State::Empty => {
                Event::AllocatedUnsealSector(_) => State::Allocated,
            },
            State::Allocated => {
                Event::UnsealDone(_) => State::Unsealed,
            },
            State::Unsealed => {
                Event::UploadPieceDone => State::Finished,
            },
        };

        Ok(next)
    }

    fn exec(&self, task: &mut Task) -> Result<Option<Event>, Failure> {
        let state = task.sector.state;
        let inner = Unseal { task };

        match state {
            State::Empty => inner.acquire_task(),
            State::Allocated => inner.unseal(),
            State::Unsealed => inner.upload_piece(),
            State::Finished => return Ok(None),

            other => {
                Err(anyhow!("unexpected state: {:?} in unseal planner", other)
                    .abort())
            }
        }
        .map(Some)
    }

    fn apply(&self, event: Event, state: State, task: &mut Task) -> Result<()> {
        event.apply(state, task)
    }
}

// empty -> acquire -> unseal -> upload -> finish

struct Unseal<'t> {
    task: &'t mut Task,
}

impl<'t> Unseal<'t> {
    fn acquire_task(&self) -> Result<Event, Failure> {
        let maybe_res = call_rpc! {
            self.task.rpc()=>allocate_unseal_sector(AllocateSectorSpec {
                allowed_miners: Some(self.task.sealing_ctrl.config().allowed_miners.clone()),
                allowed_proof_types: Some(self.task.sealing_ctrl.config().allowed_proof_types.clone()),
            },)
        };

        let maybe_allocated = match maybe_res {
            Ok(a) => a,
            Err(e) => {
                warn!(
                    "unseal sector are not allocated yet, so we can retry even though we got the err {:?}",
                    e
                );
                return Ok(Event::Idle);
            }
        };

        let allocated = match maybe_allocated {
            Some(a) => a,
            None => return Ok(Event::Idle),
        };

        Ok(Event::AllocatedUnsealSector(allocated))
    }

    fn unseal(&self) -> Result<Event, Failure> {
        // query token
        let _token = self
            .task
            .sealing_ctrl
            .ctrl_ctx()
            .wait(STAGE_NAME_UNSEAL)
            .crit()?;

        let sector_id = self.task.sector_id()?;
        let proof_type = self.task.sector_proof_type()?;

        field_required!(
            unseal_info,
            self.task.sector.phases.unseal_in.as_ref()
        );
        field_required!(
            instance_name,
            self.task
                .sector
                .finalized
                .as_ref()
                .map(|f| &f.private.access_instance)
        );

        debug!("find access store named {}", instance_name);
        let instance = self
            .task
            .sealing_ctrl
            .ctx()
            .global
            .attached
            .get(instance_name)
            .with_context(|| {
                format!("get access store instance named {}", instance_name)
            })
            .perm()?;

        let sealed_path = instance
            .path(Resource::Sealed(sector_id.clone()))
            .with_context(|| {
                format!(
                    "get path for sealed({}) in {}",
                    sector_id, instance_name
                )
            })
            .perm()?;
        let cache_path = instance
            .path(Resource::Cache(sector_id.clone()))
            .with_context(|| {
                format!(
                    "get path for cache({}) in {}",
                    sector_id, instance_name
                )
            })
            .perm()?;

        let piece_file = self.task.piece_file(&unseal_info.piece_cid);
        if piece_file.full().exists() {
            remove_file(&piece_file)
                .context("remove the existing piece file")
                .perm()?;
        } else {
            piece_file.prepare().context("prepare piece file").perm()?;
        }

        field_required! {
            prove_input,
            self.task.sector.base.as_ref().map(|b| b.prove_input)
        }
        let (prover_id, sector_id) = prove_input;

        field_required!(ticket, self.task.sector.phases.ticket.as_ref());

        // call unseal fn
        let out = self
            .task
            .sealing_ctrl
            .ctx()
            .global
            .processors
            .unseal
            .process(
                self.task.sealing_ctrl.ctrl_ctx(),
                UnsealInput {
                    registered_proof: (*proof_type).into(),
                    prover_id,
                    sector_id,
                    comm_d: unseal_info.comm_d,
                    ticket: ticket.ticket.0,
                    cache_dir: cache_path,
                    sealed_file: sealed_path,
                    unsealed_output: piece_file.into(),
                    offset: UnpaddedByteIndex(unseal_info.offset),
                    num_bytes: UnpaddedBytesAmount(unseal_info.size),
                },
            )
            .perm()?;

        debug!(
            sector =?sector_id,
            offset =? unseal_info.offset,
            size =? unseal_info.size,
            "unseal bytes amount: {:?}", out);

        Ok(Event::UnsealDone(out.0))
    }

    fn upload_piece(&self) -> Result<Event, Failure> {
        let _token = self
            .task
            .sealing_ctrl
            .ctrl_ctx()
            .wait(STAGE_NAME_TRANSFER)
            .crit()?;

        let sector_id = self.task.sector_id()?;

        field_required!(
            unseal_info,
            self.task.sector.phases.unseal_in.as_ref()
        );
        let piece_file = self.task.piece_file(&unseal_info.piece_cid);
        field_required!(
            unseal_info,
            self.task.sector.phases.unseal_in.as_ref()
        );

        // parse dest
        let dests = call_rpc! {
            self.task.rpc()=>acquire_unseal_dest(sector_id.clone(), unseal_info.piece_cid.clone(),)
        }?;

        if !dests.is_empty() {
            info!(sector =?sector_id, "get {} upload dest", dests.len());
        }

        for d in dests.into_iter() {
            let dest = &d;
            let raw_url = Url::parse(dest)
                .context(format!("parse url {}", dest))
                .perm()?;

            // we accept four kinds of url by now
            // 1. http://xxx or https://xxx , it means we will post data to a http server
            // 2. market://store_name/piece_cid, it means we will transfer data to the target path of pieces store from market
            // 3. file:///path , it means we will transfer data to a local file
            // 4. store://store_name/piece_cid , it means we will transfer data to store in damocles-manager
            match raw_url.scheme() {
                "http" | "https" => {
                    // post req
                    let fd = File::open(&piece_file)
                        .context("open piece file")
                        .temp()?;
                    let body = reqwest::blocking::Body::from(fd);

                    let resp = reqwest::blocking::Client::new()
                        .put(raw_url)
                        .body(body)
                        .send()
                        .temp()?;
                    if !resp.status().is_success() {
                        return Err(anyhow!("upload piece failed: {:?}", resp))
                            .temp();
                    }
                }
                "market" => {
                    let query =
                        raw_url.query_pairs().collect::<HashMap<_, _>>();

                    let (host, scheme, token) = match (
                        query.get("host"),
                        query.get("scheme"),
                        query.get("token"),
                    ) {
                        (Some(h), Some(s), Some(t)) => (h, s, t),
                        _ => {
                            return Err(anyhow!("parse url fail {}", raw_url))
                                .perm()
                        }
                    };

                    let piece_cid = raw_url.path().trim_matches('/');
                    let store_name = match raw_url.host_str() {
                        Some(v) => v,
                        None => {
                            return Err(anyhow!(
                                "store name not found in {}",
                                raw_url
                            ))
                            .perm()
                        }
                    };

                    let fd = File::open(&piece_file)
                        .context("open piece file")
                        .temp()?;
                    let body = reqwest::blocking::Body::from(fd);

                    // url example: market://store_name/piece_cid => http://market_ip/resource?resource-id=piece_cid&store=store_name
                    let target_url = Url::parse(&format!(
                        "{}://{}/resource?resource-id={}&store={}",
                        scheme, host, piece_cid, store_name
                    ))
                    .context(format!("parse url {}", dest))
                    .perm()?;

                    let resp = reqwest::blocking::Client::new()
                        .put(target_url)
                        .header("Authorization", format!("Bearer {}", token))
                        .body(body)
                        .send()
                        .temp()?;

                    debug!("upload piece to market, resp: {:?}", resp);
                    if resp.status() != reqwest::StatusCode::OK {
                        return Err(anyhow!(
                            "upload piece to market failed, resp: {:?}",
                            resp
                        ))
                        .perm();
                    }
                }
                "file" => {
                    // copy to dest file
                    let path_str = raw_url.path();
                    if path_str.is_empty() {
                        return Err(anyhow!("path not found in {}", raw_url))
                            .perm();
                    }
                    let des_path = Path::new(path_str);
                    fs::copy(piece_file.full(), des_path).perm()?;
                }
                "store" => {
                    let access_instance = raw_url.host_str();
                    let p = raw_url.path();
                    if p.is_empty() {
                        return Err(anyhow!("path not found in {}", raw_url))
                            .perm();
                    }
                    let des_path = p;

                    match access_instance {
                        Some(ins_name) => {
                            let ins_name = ins_name.to_string();
                            let access_store = self
                                .task
                                .sealing_ctrl
                                .ctx()
                                .global
                                .attached
                                .get(&ins_name)
                                .with_context(|| {
                                    format!(
                                        "get access store instance named {}",
                                        ins_name
                                    )
                                })
                                .perm()?;

                            debug!(
                                "get basic info for access store named {}",
                                ins_name
                            );
                            let access_store_basic_info = call_rpc! {
                                self.task.rpc()=>store_basic_info(ins_name.clone(),)
                            }?
                            .with_context(|| format!("get basic info for store named {}", ins_name))
                            .perm()?;

                            let transfer_routes = vec![TransferRoute {
                                src: TransferItem::Local(
                                    piece_file.full().clone(),
                                ),
                                dest: TransferItem::Store {
                                    store: ins_name.clone(),
                                    path: access_store
                                        .path(Resource::Custom(
                                            des_path.to_string(),
                                        ))
                                        .perm()?,
                                },
                                opt: None,
                            }];

                            let transfer = TransferInput {
                                routes: transfer_routes,
                                stores: HashMap::from_iter([(
                                    ins_name.clone(),
                                    TransferStoreInfo {
                                        name: ins_name.clone(),
                                        meta: access_store_basic_info.meta,
                                    },
                                )]),
                            };

                            self.task
                                .sealing_ctrl
                                .ctx()
                                .global
                                .processors
                                .transfer
                                .process(
                                    self.task.sealing_ctrl.ctrl_ctx(),
                                    transfer,
                                )
                                .context("link unseal sector files")
                                .perm()?;
                        }
                        None => {
                            // use remote piece store by default
                            let access_store = &self
                                .task
                                .sealing_ctrl
                                .ctx()
                                .global
                                .remote_piece_store;
                            let p = p.trim_matches('/');
                            let piece_cid = Cid::try_from(p)
                                .context(format!("parse cid {}", p))
                                .perm()?;
                            let url = match access_store.get(&piece_cid).unwrap() {
                                PieceFile::Url(u) => u,
                                _ => {
                                    return Err(anyhow!(
                                        "unexpected piece_file  in remote piece store"
                                    ))
                                    .perm()
                                }
                            };

                            let fd = File::open(&piece_file)
                                .context("open piece file")
                                .temp()?;
                            let body = reqwest::blocking::Body::from(fd);
                            let resp = reqwest::blocking::Client::new()
                                .put(url)
                                .body(body)
                                .send()
                                .temp()?;
                            if !resp.status().is_success() {
                                let resp_info = format!("{:?}", &resp);
                                let error_info = resp.text().temp()?;
                                return Err(anyhow!(
                                    "upload piece failed: {}, body({})",
                                    resp_info,
                                    error_info
                                ))
                                .temp();
                            }
                        }
                    }
                }
                _ => {
                    return Err(anyhow!(
                        "unsupported url scheme {}",
                        raw_url.scheme()
                    ))
                    .perm();
                }
            }

            info!(
                "upload piece done, piece_cid: {}, dest: {}",
                unseal_info.piece_cid.0, dest
            );
        }

        call_rpc! {
            self.task.rpc()=>achieve_unseal_sector(sector_id.clone(), unseal_info.piece_cid.clone(), "".to_string(),)
        }?;

        Ok(Event::UploadPieceDone)
    }
}
