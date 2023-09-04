use std::{
    collections::HashMap,
    fs::{self, create_dir_all, remove_dir_all, remove_file},
    os::unix::fs::symlink,
    path::Path,
};

use anyhow::{anyhow, Context, Result};
use vc_processors::builtin::tasks::{
    Piece, PieceFile, STAGE_NAME_ADD_PIECES, STAGE_NAME_TREED,
};

use crate::{
    rpc::sealer::{Deals, SectorID, Seed, Ticket},
    sealing::{
        failure::{Failure, IntoFailure, MapErrToFailure, MapStdErrToFailure},
        processor::{
            cached_filenames_for_sector, seal_commit_phase1,
            snap_generate_partition_proofs, snap_verify_sector_update_proof,
            tree_d_path_in_dir, AddPiecesInput, PC1Input, PC2Input, PieceInfo,
            SealCommitPhase1Output, SealPreCommitPhase1Output,
            SealPreCommitPhase2Output, SnapEncodeInput, SnapEncodeOutput,
            SnapProveInput, SnapProveOutput, TransferInput, TransferItem,
            TransferRoute, TransferStoreInfo, TreeDInput, UnpaddedBytesAmount,
            STAGE_NAME_C1, STAGE_NAME_PC1, STAGE_NAME_PC2,
            STAGE_NAME_SNAP_ENCODE, STAGE_NAME_SNAP_PROVE,
        },
        sealing_thread::{
            entry::Entry,
            util::{call_rpc, cloned_required, field_required},
        },
    },
    types::SIZE_32G,
    SealProof,
};

use super::task::Task;

pub(crate) fn add_pieces(
    task: &Task,
    deals: &Deals,
) -> Result<Vec<PieceInfo>, Failure> {
    let _token = task
        .sealing_ctrl
        .ctrl_ctx()
        .wait(STAGE_NAME_ADD_PIECES)
        .crit()?;

    let seal_proof_type = task.sector_proof_type()?.into();
    let staged_filepath = task.staged_file(task.sector_id()?);
    if staged_filepath.full().exists() {
        remove_file(&staged_filepath)
            .context("remove the existing staged file")
            .perm()?;
    } else {
        staged_filepath
            .prepare()
            .context("prepare staged file")
            .perm()?;
    }

    let piece_store = task.sealing_ctrl.ctx().global.piece_store.as_ref();

    let mut pieces = Vec::with_capacity(deals.len());

    for deal in deals {
        let unpadded_piece_size = deal.piece.size.unpadded();
        let is_pledged = deal.id == 0;

        let piece_file: PieceFile = if is_pledged {
            PieceFile::Pledge
        } else {
            piece_store
                .get(&deal.piece.cid.0)
                .with_context(|| format!("get piece: {}", deal.piece.cid.0))
                .perm()?
        };
        pieces.push(Piece {
            piece_file,
            payload_size: deal.payload_size,
            piece_size: UnpaddedBytesAmount(unpadded_piece_size.0),
        })
    }

    task.sealing_ctrl
        .ctx()
        .global
        .processors
        .add_pieces
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            AddPiecesInput {
                seal_proof_type,
                pieces,
                staged_filepath: staged_filepath.into(),
            },
        )
        .context("add pieces")
        .perm()
}

// build tree_d inside `prepare_dir` if necessary
pub(crate) fn build_tree_d(
    task: &Task,
    allow_static: bool,
) -> Result<(), Failure> {
    let sector_id = task.sector_id()?;
    let proof_type = task.sector_proof_type()?;

    let _token = task.sealing_ctrl.ctrl_ctx().wait(STAGE_NAME_TREED).crit()?;

    let prepared_dir = task.prepared_dir(sector_id);
    prepared_dir.prepare().perm()?;

    let tree_d_path = tree_d_path_in_dir(prepared_dir.as_ref());
    if tree_d_path.exists() {
        remove_file(&tree_d_path)
            .with_context(|| {
                format!("cleanup preprared tree d file {:?}", tree_d_path)
            })
            .crit()?;
    }

    // pledge sector
    if allow_static
        && task.sector.deals.as_ref().map(|d| d.len()).unwrap_or(0) == 0
    {
        if let Some(static_tree_path) = task
            .sealing_ctrl
            .ctx()
            .global
            .static_tree_d
            .get(&proof_type.sector_size())
        {
            symlink(
                static_tree_path,
                tree_d_path_in_dir(prepared_dir.as_ref()),
            )
            .crit()?;
            return Ok(());
        }
    }

    let staged_file = task.staged_file(sector_id);

    task.sealing_ctrl
        .ctx()
        .global
        .processors
        .tree_d
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            TreeDInput {
                registered_proof: (*proof_type).into(),
                staged_file: staged_file.into(),
                cache_dir: prepared_dir.into(),
            },
        )
        .perm()?;

    Ok(())
}

fn cleanup_before_pc1(cache_dir: &Entry, sealed_file: &Entry) -> Result<()> {
    // TODO: see if we have more graceful ways to handle restarting pc1
    let cache_dir_path: &Path = cache_dir.as_ref();
    if cache_dir_path.exists() {
        remove_dir_all(cache_dir)?;
    }
    create_dir_all(cache_dir_path)?;
    tracing::debug!("init cache dir {:?} before pc1", cache_dir_path);

    let _ = sealed_file.init_file()?;
    tracing::debug!("truncate sealed file {:?} before pc1", sealed_file);

    Ok(())
}

pub(crate) fn pre_commit1(
    task: &Task,
) -> Result<(Ticket, SealPreCommitPhase1Output), Failure> {
    let _token = task.sealing_ctrl.ctrl_ctx().wait(STAGE_NAME_PC1).crit()?;

    let sector_id = task.sector_id()?;
    let proof_type = task.sector_proof_type()?;

    let ticket = match &task.sector.phases.ticket {
        // Use the existing ticket when rebuilding sectors
        Some(ticket) => ticket.clone(),
        None => {
            let ticket = call_rpc! {
                task.rpc() => assign_ticket(sector_id.clone(),)
            }?;
            tracing::debug!(ticket = ?ticket.ticket.0, epoch = ticket.epoch, "ticket assigned from sector-manager");
            ticket
        }
    };

    field_required! {
        piece_infos,
        task.sector.phases.pieces.as_ref().cloned()
    }

    let cache_dir = task.cache_dir(sector_id);
    let staged_file = task.staged_file(sector_id);
    let sealed_file = task.sealed_file(sector_id);
    let prepared_dir = task.prepared_dir(sector_id);

    cleanup_before_pc1(&cache_dir, &sealed_file)
        .context("cleanup before pc1")
        .crit()?;
    symlink(
        tree_d_path_in_dir(prepared_dir.as_ref()),
        tree_d_path_in_dir(cache_dir.as_ref()),
    )
    .crit()?;

    field_required! {
        prove_input,
        task.sector.base.as_ref().map(|b| b.prove_input)
    }

    let out = task
        .sealing_ctrl
        .ctx()
        .global
        .processors
        .pc1
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            PC1Input {
                registered_proof: (*proof_type).into(),
                cache_path: cache_dir.into(),
                in_path: staged_file.into(),
                out_path: sealed_file.into(),
                prover_id: prove_input.0,
                sector_id: prove_input.1,
                ticket: ticket.ticket.0,
                piece_infos,
            },
        )
        .perm()?;

    Ok((ticket, out))
}

fn cleanup_before_pc2(cache_dir: &Path) -> Result<()> {
    for entry_res in cache_dir.read_dir()? {
        let entry = entry_res?;
        let fname = entry.file_name();
        if let Some(fname_str) = fname.to_str() {
            let should = fname_str == "p_aux"
                || fname_str == "t_aux"
                || fname_str.contains("tree-c")
                || fname_str.contains("tree-r-last");

            if !should {
                continue;
            }

            let p = entry.path();
            remove_file(&p)
                .with_context(|| format!("remove cached file {:?}", p))?;
            tracing::debug!("remove cached file {:?} before pc2", p);
        }
    }

    Ok(())
}

pub(crate) fn pre_commit2(
    task: &'_ Task,
) -> Result<SealPreCommitPhase2Output, Failure> {
    let _token = task.sealing_ctrl.ctrl_ctx().wait(STAGE_NAME_PC2).crit()?;

    let sector_id = task.sector_id()?;

    field_required! {
        pc1out,
        task.sector.phases.pc1out.as_ref().cloned()
    }

    let cache_dir = task.cache_dir(sector_id);
    let sealed_file = task.sealed_file(sector_id);

    cleanup_before_pc2(cache_dir.as_ref()).crit()?;

    let pc2_running_file_entry = task.pc2_running_file(sector_id);
    let pc2_running_file = pc2_running_file_entry.full();
    if pc2_running_file.exists() {
        // The pc2 task will read the contents of the sealed file and modify it.
        // If pc2 is restarted halfway, the contents of the sealed file will be modified.
        // When the pc2 task is restarted again, the pc2 result will be wrong.
        // So we add the .pc2_running file. If this file exists when pc2 start,
        // it means that pc2 has been restarted before and the sealed file needs to be copied again to ensure the correctness of the sealed file.
        fs::remove_file(sealed_file.full())
            .context("remove sealed file")
            .crit()?;
        fs::copy(task.staged_file(sector_id).full(), sealed_file.full())
            .context("copy sealed file")
            .crit()?;
    } else {
        fs::File::create(pc2_running_file)
            .context("create pc2 running file")
            .crit()?;
    }

    let out = task
        .sealing_ctrl
        .ctx()
        .global
        .processors
        .pc2
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            PC2Input {
                pc1out,
                cache_dir: cache_dir.into(),
                sealed_file: sealed_file.into(),
            },
        )
        .perm()?;

    fs::remove_file(pc2_running_file)
        .context("remove pc2 running file")
        .crit()?;
    Ok(out)
}

pub(crate) fn commit1_with_seed(
    task: &Task,
    seed: Seed,
) -> Result<SealCommitPhase1Output, Failure> {
    let _token = task.sealing_ctrl.ctrl_ctx().wait(STAGE_NAME_C1).crit()?;

    let sector_id = task.sector_id()?;

    field_required! {
        prove_input,
        task.sector.base.as_ref().map(|b| b.prove_input)
    }

    let (seal_prover_id, seal_sector_id) = prove_input;

    field_required! {
        piece_infos,
        task.sector.phases.pieces.as_ref()
    }

    field_required! {
        ticket,
        task.sector.phases.ticket.as_ref()
    }

    cloned_required! {
        p2out,
        task.sector.phases.pc2out
    }

    let cache_dir = task.cache_dir(sector_id);
    let sealed_file = task.sealed_file(sector_id);

    let out = seal_commit_phase1(
        cache_dir.into(),
        sealed_file.into(),
        seal_prover_id,
        seal_sector_id,
        ticket.ticket.0,
        seed.seed.0,
        p2out,
        piece_infos,
    )
    .perm()?;

    Ok(out)
}

pub(crate) fn snap_encode(
    task: &Task,
    sector_id: &SectorID,
    proof_type: &SealProof,
) -> Result<SnapEncodeOutput, Failure> {
    let _token = task
        .sealing_ctrl
        .ctrl_ctx()
        .wait(STAGE_NAME_SNAP_ENCODE)
        .crit()?;

    cloned_required!(piece_infos, task.sector.phases.pieces);

    // sealed file & cache dir should be ready now, we just do nothing here
    let sealed_file = task.sealed_file(sector_id);
    let cache_dir = task.cache_dir(sector_id);

    // init update file
    let update_file = task.update_file(sector_id);
    tracing::debug!(path=?update_file.full(),  "trying to init update file");
    {
        let file = update_file.init_file().perm()?;
        file.set_len(proof_type.sector_size())
            .context("fallocate for update file")
            .perm()?;
    }

    let update_cache_dir = task.update_cache_dir(sector_id);
    tracing::debug!(path=?update_cache_dir.full(),  "trying to init update cache dir");
    update_cache_dir
        .prepare()
        .context("prepare update cache dir")
        .perm()?;

    // tree d
    tracing::debug!("trying to prepare tree_d");
    let prepared_dir = task.prepared_dir(sector_id);
    let tree_d_link = tree_d_path_in_dir(update_cache_dir.as_ref());
    if tree_d_link.exists() {
        fs::remove_file(&tree_d_link)
            .context("remove existing tree_d link file")
            .crit()?;
    }
    symlink(tree_d_path_in_dir(prepared_dir.as_ref()), tree_d_link)
        .context("link prepared tree_d")
        .crit()?;
    // staged file should be already exists, do nothing
    let staged_file = task.staged_file(sector_id);

    task.sealing_ctrl
        .ctx()
        .global
        .processors
        .snap_encode
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            SnapEncodeInput {
                registered_proof: proof_type.into(),
                new_replica_path: update_file.into(),
                new_cache_path: update_cache_dir.into(),
                sector_path: sealed_file.into(),
                sector_cache_path: cache_dir.into(),
                staged_data_path: staged_file.into(),
                piece_infos,
            },
        )
        .perm()
}

pub(crate) fn snap_prove(task: &Task) -> Result<SnapProveOutput, Failure> {
    let _token = task
        .sealing_ctrl
        .ctrl_ctx()
        .wait(STAGE_NAME_SNAP_PROVE)
        .crit()?;

    let sector_id = task.sector_id()?;
    let proof_type = task.sector_proof_type()?;
    field_required!(encode_out, task.sector.phases.snap_encode_out.as_ref());
    field_required!(
        comm_r_old,
        task.sector.finalized.as_ref().map(|f| f.public.comm_r)
    );

    let sealed_file = task.sealed_file(sector_id);
    let cached_dir = task.cache_dir(sector_id);
    let update_file = task.update_file(sector_id);
    let update_cache_dir = task.update_cache_dir(sector_id);

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

    let proof = task
        .sealing_ctrl
        .ctx()
        .global
        .processors
        .snap_prove
        .process(
            task.sealing_ctrl.ctrl_ctx(),
            SnapProveInput {
                registered_proof: (*proof_type).into(),
                vannilla_proofs: vannilla_proofs
                    .into_iter()
                    .map(|b| b.0)
                    .collect(),
                comm_r_old,
                comm_r_new: encode_out.comm_r_new,
                comm_d_new: encode_out.comm_d_new,
            },
        )
        .perm()?;

    let verified = snap_verify_sector_update_proof(
        proof_type.into(),
        &proof,
        comm_r_old,
        encode_out.comm_r_new,
        encode_out.comm_d_new,
    )
    .perm()?;

    if !verified {
        return Err(anyhow!("generated an invalid update proof").perm());
    }

    Ok(proof)
}

// acquire a persist store for sector files, copy the files and return the instance name of the
// acquired store
pub(crate) fn persist_sector_files(
    task: &Task,
    cache_dir: Entry,
    sealed_file: Entry,
) -> Result<String, Failure> {
    let sector_id = task.sector_id()?;
    let proof_type = task.sector_proof_type()?;
    let sector_size = proof_type.sector_size();
    // 1.02 * sector size
    let required_size = if sector_size < SIZE_32G {
        // 2x sector size for sector smaller than 32GiB
        sector_size * 2
    } else {
        // 1.02x sector size for 32GiB, 64GiB
        sector_size + sector_size / 50
    };

    let candidates = task
        .sealing_ctrl
        .ctx()
        .global
        .attached
        .available_instances();
    if candidates.is_empty() {
        return Err(anyhow!("no available local persist store candidate"))
            .perm();
    }

    let ins_info = loop {
        let res = call_rpc! {
            task.rpc() => store_reserve_space(sector_id.clone(), required_size, candidates.clone(),)
        }?;

        if let Some(selected) = res {
            break selected;
        }

        tracing::warn!(
            required_size=required_size,
            sector_id=?sector_id,
            candidates=?candidates,
            "no persist store selected, wait for next polling"
        );
        task.sealing_ctrl.wait_or_interrupted(
            task.sealing_ctrl.config().rpc_polling_interval,
        )?;
    };

    let persist_store = task
        .sealing_ctrl
        .ctx()
        .global
        .attached
        .get(&ins_info.name)
        .context("no available persist store")
        .perm()?;

    let ins_name = persist_store.instance();
    tracing::debug!(name = %ins_name, "persist store acquired");

    let mut wanted = vec![sealed_file];
    wanted.extend(
        cached_filenames_for_sector(proof_type.into())
            .into_iter()
            .map(|fname| cache_dir.join(fname)),
    );

    let transfer_routes = wanted
        .into_iter()
        .map(|p| {
            let rel_path = p.rel();
            Ok(TransferRoute {
                // local
                src: TransferItem {
                    store_name: None,
                    uri: p.full().to_owned(),
                },
                // persist store
                dest: TransferItem {
                    store_name: Some(ins_name.clone()),
                    uri: persist_store
                        .uri(rel_path)
                        .with_context(|| format!("get uri for {:?}", rel_path))
                        .perm()?,
                },
                opt: None,
            })
        })
        .collect::<Result<Vec<_>, Failure>>()?;

    let transfer_store_info = TransferStoreInfo {
        name: ins_name.clone(),
        meta: ins_info.meta,
    };

    let transfer = TransferInput {
        stores: HashMap::from_iter([(ins_name.clone(), transfer_store_info)]),
        routes: transfer_routes,
    };

    task.sealing_ctrl
        .ctx()
        .global
        .processors
        .transfer
        .process(task.sealing_ctrl.ctrl_ctx(), transfer)
        .context("transfer persist sector files")
        .perm()?;

    Ok(ins_name)
}

pub(crate) fn submit_persisted(
    task: &Task,
    is_upgrade: bool,
) -> Result<(), Failure> {
    let sector_id = task.sector_id()?;

    field_required! {
        instance,
        task.sector.phases.persist_instance.as_ref().cloned()
    }

    let checked = call_rpc! {
        task.rpc() => submit_persisted_ex(sector_id.clone(), instance,is_upgrade,)
    }?;

    if checked {
        Ok(())
    } else {
        Err(anyhow!(
            "sector files are persisted but unavailable for sealer"
        ))
        .perm()
    }
}
