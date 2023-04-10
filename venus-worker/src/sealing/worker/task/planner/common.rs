//! this module provides some common handlers

use std::collections::HashMap;
use std::fs::{create_dir_all, remove_dir_all, remove_file};
use std::os::unix::fs::symlink;
use std::path::Path;
use tracing::debug;

use anyhow::{anyhow, Context, Result};
use vc_fil_consumers::tasks::{AddPieces, Piece, PieceFile, SnapEncode, SnapProve, Transfer, TreeD, PC1, PC2};

use super::super::{call_rpc, cloned_required, field_required, Entry, Task};
use crate::rpc::sealer::{Deals, SectorID, Seed, Ticket};
use crate::sealing::failure::*;
use crate::sealing::processor::{
    cached_filenames_for_sector, seal_commit_phase1, snap_generate_partition_proofs, snap_verify_sector_update_proof, tree_d_path_in_dir,
    Client, PieceInfo, SealCommitPhase1Output, SealPreCommitPhase1Output, SealPreCommitPhase2Output, SnapEncodeOutput, SnapProveOutput,
    TransferItem, TransferRoute, TransferStoreInfo, UnpaddedBytesAmount,
};
use crate::types::{SealProof, SIZE_32G};

impl Task<'_> {
    pub fn add_pieces(&mut self, deals: Deals) -> Result<Vec<PieceInfo>, Failure> {
        let seal_proof_type = self.sector_proof_type()?.into();
        let staged_filepath = self.staged_file(self.sector_id()?);
        if staged_filepath.full().exists() {
            remove_file(&staged_filepath).context("remove the existing staged file").perm()?;
        } else {
            staged_filepath.prepare().context("prepare staged file").perm()?;
        }

        let piece_store = self.ctx.global.piece_store.as_ref();

        let mut pieces = Vec::with_capacity(deals.len());

        for deal in deals {
            let unpadded_piece_size = deal.piece.size.unpadded();
            let is_pledged = deal.id == 0;

            let piece_file = if is_pledged {
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

        self.processors
            .add_pieces
            .process(AddPieces {
                seal_proof_type,
                pieces,
                staged_filepath: staged_filepath.into(),
            })
            .context("add pieces")
            .perm()
    }

    // build tree_d inside `prepare_dir` if necessary
    pub fn build_tree_d(&mut self, allow_static: bool) -> Result<(), Failure> {
        let sector_id = self.sector_id()?;
        let proof_type = self.sector_proof_type()?;

        let prepared_dir = self.prepared_dir(sector_id);
        prepared_dir.prepare().perm()?;

        let tree_d_path = tree_d_path_in_dir(prepared_dir.as_ref());
        if tree_d_path.exists() {
            remove_file(&tree_d_path)
                .with_context(|| format!("cleanup preprared tree d file {:?}", tree_d_path))
                .crit()?;
        }

        // pledge sector
        if allow_static && self.sector.deals.as_ref().map(|d| d.len()).unwrap_or(0) == 0 {
            if let Some(static_tree_path) = self.ctx.global.static_tree_d.get(&proof_type.sector_size()) {
                symlink(static_tree_path, tree_d_path_in_dir(prepared_dir.as_ref())).crit()?;
                return Ok(());
            }
        }

        let staged_file = self.staged_file(sector_id);

        self.processors
            .tree_d
            .process(TreeD {
                registered_proof: (*proof_type).into(),
                staged_file: staged_file.into(),
                cache_dir: prepared_dir.into(),
            })
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
        debug!("init cache dir {:?} before pc1", cache_dir_path);

        let _ = sealed_file.init_file()?;
        debug!("truncate sealed file {:?} before pc1", sealed_file);

        Ok(())
    }

    pub fn pre_commit1(&mut self) -> Result<(Ticket, SealPreCommitPhase1Output), Failure> {
        let sector_id = self.sector_id()?;
        let proof_type = self.sector_proof_type()?;

        let ticket = match &self.sector.phases.ticket {
            // Use the existing ticket when rebuilding sectors
            Some(ticket) => ticket.clone(),
            None => {
                let ticket = call_rpc! {
                    self.ctx.global.rpc,
                    assign_ticket,
                    sector_id.clone(),
                }?;
                debug!(ticket = ?ticket.ticket.0, epoch = ticket.epoch, "ticket assigned from sector-manager");
                ticket
            }
        };

        field_required! {
            piece_infos,
            self.sector.phases.pieces.as_ref().cloned()
        }

        let cache_dir = self.cache_dir(sector_id);
        let staged_file = self.staged_file(sector_id);
        let sealed_file = self.sealed_file(sector_id);
        let prepared_dir = self.prepared_dir(sector_id);

        Self::cleanup_before_pc1(&cache_dir, &sealed_file)
            .context("cleanup before pc1")
            .crit()?;
        symlink(tree_d_path_in_dir(prepared_dir.as_ref()), tree_d_path_in_dir(cache_dir.as_ref())).crit()?;

        field_required! {
            prove_input,
            self.sector.base.as_ref().map(|b| b.prove_input)
        }

        let out = self
            .processors
            .pc1
            .process(PC1 {
                registered_proof: (*proof_type).into(),
                cache_path: cache_dir.into(),
                in_path: staged_file.into(),
                out_path: sealed_file.into(),
                prover_id: prove_input.0,
                sector_id: prove_input.1,
                ticket: ticket.ticket.0,
                piece_infos,
            })
            .perm()?;

        Ok((ticket, out))
    }

    fn cleanup_before_pc2(cache_dir: &Path) -> Result<()> {
        for entry_res in cache_dir.read_dir()? {
            let entry = entry_res?;
            let fname = entry.file_name();
            if let Some(fname_str) = fname.to_str() {
                let should =
                    fname_str == "p_aux" || fname_str == "t_aux" || fname_str.contains("tree-c") || fname_str.contains("tree-r-last");

                if !should {
                    continue;
                }

                let p = entry.path();
                remove_file(&p).with_context(|| format!("remove cached file {:?}", p))?;
                debug!("remove cached file {:?} before pc2", p);
            }
        }

        Ok(())
    }

    pub fn pre_commit2(&mut self) -> Result<SealPreCommitPhase2Output, Failure> {
        let sector_id = self.sector_id()?;

        field_required! {
            pc1out,
            self.sector.phases.pc1out.as_ref().cloned()
        }

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

        Self::cleanup_before_pc2(cache_dir.as_ref()).crit()?;

        let out = self
            .processors
            .pc2
            .process(PC2 {
                pc1out,
                cache_dir: cache_dir.into(),
                sealed_file: sealed_file.into(),
            })
            .perm()?;

        Ok(out)
    }

    pub fn commit1_with_seed(&self, seed: Seed) -> Result<SealCommitPhase1Output, Failure> {
        let sector_id = self.sector_id()?;

        field_required! {
            prove_input,
            self.sector.base.as_ref().map(|b| b.prove_input)
        }

        let (seal_prover_id, seal_sector_id) = prove_input;

        field_required! {
            piece_infos,
            self.sector.phases.pieces.as_ref()
        }

        field_required! {
            ticket,
            self.sector.phases.ticket.as_ref()
        }

        cloned_required! {
            p2out,
            self.sector.phases.pc2out
        }

        let cache_dir = self.cache_dir(sector_id);
        let sealed_file = self.sealed_file(sector_id);

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

    pub fn snap_encode(&mut self, sector_id: SectorID, proof_type: SealProof) -> Result<SnapEncodeOutput, Failure> {
        cloned_required!(piece_infos, self.sector.phases.pieces);

        // sealed file & cache dir should be ready now, we just do nothing here
        let sealed_file = self.sealed_file(&sector_id);
        let cache_dir = self.cache_dir(&sector_id);

        // init update file
        let update_file = self.update_file(&sector_id);
        debug!(path=?update_file.full(),  "trying to init update file");
        {
            let file = update_file.init_file().perm()?;
            file.set_len(proof_type.sector_size()).context("fallocate for update file").perm()?;
        }

        let update_cache_dir = self.update_cache_dir(&sector_id);
        debug!(path=?update_cache_dir.full(),  "trying to init update cache dir");
        update_cache_dir.prepare().context("prepare update cache dir").perm()?;

        // tree d
        debug!("trying to prepare tree_d");
        let prepared_dir = self.prepared_dir(&sector_id);
        symlink(
            tree_d_path_in_dir(prepared_dir.as_ref()),
            tree_d_path_in_dir(update_cache_dir.as_ref()),
        )
        .context("link prepared tree_d")
        .crit()?;

        // staged file should be already exists, do nothing
        let staged_file = self.staged_file(&sector_id);

        self.processors
            .snap_encode
            .process(SnapEncode {
                registered_proof: proof_type.into(),
                new_replica_path: update_file.into(),
                new_cache_path: update_cache_dir.into(),
                sector_path: sealed_file.into(),
                sector_cache_path: cache_dir.into(),
                staged_data_path: staged_file.into(),
                piece_infos,
            })
            .perm()
    }

    pub fn snap_prove(&mut self) -> Result<SnapProveOutput, Failure> {
        let sector_id = self.sector_id()?;
        let proof_type = *self.sector_proof_type()?;
        field_required!(encode_out, self.sector.phases.snap_encode_out.as_ref());
        field_required!(comm_r_old, self.sector.finalized.as_ref().map(|f| f.public.comm_r));

        let sealed_file = self.sealed_file(sector_id);
        let cached_dir = self.cache_dir(sector_id);
        let update_file = self.update_file(sector_id);
        let update_cache_dir = self.update_cache_dir(sector_id);

        let vannilla_proofs = snap_generate_partition_proofs(
            proof_type.into(),
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
            .processors
            .snap_prove
            .process(SnapProve {
                registered_proof: proof_type.into(),
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

        Ok(proof)
    }

    // acquire a persist store for sector files, copy the files and return the instance name of the
    // acquired store
    pub fn persist_sector_files(&mut self, cache_dir: Entry, sealed_file: Entry) -> Result<String, Failure> {
        let sector_id = self.sector_id()?;
        let proof_type = self.sector_proof_type()?;
        let sector_size = proof_type.sector_size();
        // 1.02 * sector size
        let required_size = if sector_size < SIZE_32G {
            // 2x sector size for sector smaller than 32GiB
            sector_size * 2
        } else {
            // 1.02x sector size for 32GiB, 64GiB
            sector_size + sector_size / 50
        };

        let candidates = self.ctx.global.attached.available_instances();
        if candidates.is_empty() {
            return Err(anyhow!("no available local persist store candidate")).perm();
        }

        let ins_info = loop {
            let res = call_rpc! {
                self.ctx.global.rpc,
                store_reserve_space,
                sector_id.clone(),
                required_size,
                candidates.clone(),
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
            self.wait_or_interrupted(self.store.config.rpc_polling_interval)?;
        };

        let persist_store = self
            .ctx
            .global
            .attached
            .get(&ins_info.name)
            .context("no available persist store")
            .perm()?;

        let ins_name = persist_store.instance();
        debug!(name = %ins_name, "persist store acquired");

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

        let transfer = Transfer {
            stores: HashMap::from_iter([(ins_name.clone(), transfer_store_info)]),
            routes: transfer_routes,
        };

        self.processors
            .transfer
            .process(transfer)
            .context("transfer persist sector files")
            .perm()?;

        Ok(ins_name)
    }

    pub fn submit_persisted(&self, is_upgrade: bool) -> Result<(), Failure> {
        let sector_id = self.sector_id()?;

        field_required! {
            instance,
            self.sector.phases.persist_instance.as_ref().cloned()
        }

        let checked = call_rpc! {
            self.ctx.global.rpc,
            submit_persisted_ex,
            sector_id.clone(),
            instance,
            is_upgrade,
        }?;

        if checked {
            Ok(())
        } else {
            Err(anyhow!("sector files are persisted but unavailable for sealer")).perm()
        }
    }
}
