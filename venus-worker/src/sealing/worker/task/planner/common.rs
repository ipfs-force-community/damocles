//! this module provides some common handlers

use std::collections::HashMap;
use std::fs::{remove_file, File};
use std::io::{self, prelude::*};
use std::os::unix::fs::symlink;

use anyhow::{anyhow, Context};
use vc_processors::builtin::tasks::STAGE_NAME_TREED;

use super::super::{call_rpc, Entry, Task};
use crate::logging::debug;
use crate::rpc::sealer::Deals;
use crate::sealing::failure::*;
use crate::sealing::processor::{
    tree_d_path_in_dir, write_and_preprocess, PieceInfo, RegisteredSealProof, TranferItem, TransferInput, TransferRoute, TransferStoreInfo,
    TreeDInput, UnpaddedBytesAmount,
};
use crate::types::SIZE_32G;

pub fn add_pieces<'t>(
    task: &'t Task<'_>,
    seal_proof_type: RegisteredSealProof,
    mut staged_file: &mut File,
    deals: &Deals,
) -> Result<Vec<PieceInfo>, Failure> {
    let piece_store = task.ctx.global.piece_store.as_ref().context("piece store is required").perm()?;

    let mut pieces = Vec::new();

    for deal in deals {
        debug!(deal_id = deal.id, cid = %deal.piece.cid.0, payload_size = deal.payload_size, piece_size = deal.piece.size.0, "trying to add piece");

        let unpadded_piece_size = deal.piece.size.unpadded();
        let is_pledged = deal.id == 0;
        let (piece_info, _) = if is_pledged {
            let mut pledge_piece = io::repeat(0).take(unpadded_piece_size.0);
            write_and_preprocess(
                seal_proof_type,
                &mut pledge_piece,
                &mut staged_file,
                UnpaddedBytesAmount(unpadded_piece_size.0),
            )
            .with_context(|| format!("write pledge piece, size={}", unpadded_piece_size.0))
            .perm()?
        } else {
            let mut piece_reader = piece_store.get(deal.piece.cid.0, deal.payload_size, unpadded_piece_size).perm()?;

            write_and_preprocess(
                seal_proof_type,
                &mut piece_reader,
                &mut staged_file,
                UnpaddedBytesAmount(unpadded_piece_size.0),
            )
            .with_context(|| format!("write deal piece, cid={}, size={}", deal.piece.cid.0, unpadded_piece_size.0))
            .perm()?
        };

        pieces.push(piece_info);
    }

    Ok(pieces)
}

// build tree_d inside `prepare_dir` if necessary
pub fn build_tree_d(task: &'_ Task<'_>, allow_static: bool) -> Result<(), Failure> {
    let sector_id = task.sector_id()?;
    let proof_type = task.sector_proof_type()?;

    let token = task.ctx.global.limit.acquire(STAGE_NAME_TREED).crit()?;

    let prepared_dir = task.prepared_dir(sector_id);
    prepared_dir.prepare().perm()?;

    let tree_d_path = tree_d_path_in_dir(prepared_dir.as_ref());
    if tree_d_path.exists() {
        remove_file(&tree_d_path)
            .with_context(|| format!("cleanup preprared tree d file {:?}", tree_d_path))
            .crit()?;
    }

    // pledge sector
    if allow_static && task.sector.deals.as_ref().map(|d| d.len()).unwrap_or(0) == 0 {
        if let Some(static_tree_path) = task.ctx.global.static_tree_d.get(&proof_type.sector_size()) {
            symlink(static_tree_path, tree_d_path_in_dir(prepared_dir.as_ref())).crit()?;
            return Ok(());
        }
    }

    let staged_file = task.staged_file(sector_id);

    task.ctx
        .global
        .processors
        .tree_d
        .process(TreeDInput {
            registered_proof: (*proof_type).into(),
            staged_file: staged_file.into(),
            cache_dir: prepared_dir.into(),
        })
        .perm()?;

    drop(token);
    Ok(())
}

// acquire a persist store for sector files, copy the files and return the instance name of the
// acquired store
pub fn persist_sector_files(task: &'_ Task<'_>, cache_dir: Entry, sealed_file: Entry) -> Result<String, Failure> {
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

    let candidates = task.ctx.global.attached.available_instances();
    if candidates.is_empty() {
        return Err(anyhow!("no available local persist store candidate")).perm();
    }

    let ins_info = loop {
        let res = call_rpc! {
            task.ctx.global.rpc,
            store_reserve_space,
            sector_id.clone(),
            required_size,
            candidates.clone(),
        }?;

        if let Some(selected) = res {
            break selected;
        }

        debug!("no persist store selected, wait for next polling");
        task.wait_or_interruptted(task.store.config.rpc_polling_interval)?;
    };

    let persist_store = task
        .ctx
        .global
        .attached
        .get(&ins_info.name)
        .context("no available persist store")
        .perm()?;

    let ins_name = persist_store.instance();
    debug!(name = %ins_name, "persist store acquired");

    let mut wanted = vec![sealed_file];

    // here we treat fs err as temp
    for entry_res in cache_dir.read_dir().temp()? {
        let entry = entry_res.temp()?;
        if let Some(fname_str) = entry.rel().file_name().and_then(|name| name.to_str()) {
            let should = fname_str == "p_aux" || fname_str == "t_aux" || fname_str.contains("tree-r-last");

            if !should {
                continue;
            }

            wanted.push(entry);
        }
    }

    let transfer_routes = wanted
        .into_iter()
        .map(|p| {
            let rel_path = p.rel();
            Ok(TransferRoute {
                // local
                src: TranferItem {
                    store_name: None,
                    uri: p.full().to_owned(),
                },
                dest: TranferItem {
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
        loc: persist_store.loc(),
        meta: ins_info.meta,
    };

    let transfer = TransferInput {
        stores: HashMap::from_iter([(persist_store.instance(), transfer_store_info)]),
        routes: transfer_routes,
    };

    task.ctx.global.processors.transfer.process(transfer).perm()?;

    Ok(ins_name)
}
