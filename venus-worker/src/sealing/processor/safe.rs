use std::panic::catch_unwind;
use std::path::PathBuf;

use anyhow::Result;
use filecoin_proofs_api::seal;
pub use filecoin_proofs_api::seal::{
    add_piece, clear_cache, Labels, SealCommitPhase1Output, SealCommitPhase2Output,
    SealPreCommitPhase1Output, SealPreCommitPhase2Output,
};
pub use filecoin_proofs_api::{
    Commitment, PaddedBytesAmount, PieceInfo, ProverId, RegisteredSealProof, SectorId, Ticket,
    UnpaddedBytesAmount,
};

macro_rules! safe_call {
    ($ex:expr) => {
        match catch_unwind(move || $ex.map_err(|e| format!("{:?}", e))) {
            Ok(r) => r.map_err(anyhow::Error::msg),
            Err(p) => {
                let error_msg = match p.downcast_ref::<&'static str>() {
                    Some(message) => message,
                    _ => "no unwind information",
                };
                Err(anyhow::Error::msg(error_msg))
            }
        }
    };
}

pub fn seal_commit_phase1(
    cache_path: PathBuf,
    replica_path: PathBuf,
    prover_id: ProverId,
    sector_id: SectorId,
    ticket: Ticket,
    seed: Ticket,
    pre_commit: SealPreCommitPhase2Output,
    piece_infos: &[PieceInfo],
) -> Result<SealCommitPhase1Output> {
    safe_call! {
        seal::seal_commit_phase1(
            cache_path,
            replica_path,
            prover_id,
            sector_id,
            ticket,
            seed,
            pre_commit,
            piece_infos,
        )
    }
}

pub fn seal_commit_phase2(
    phase1_output: SealCommitPhase1Output,
    prover_id: ProverId,
    sector_id: SectorId,
) -> Result<SealCommitPhase2Output> {
    safe_call! {
        seal::seal_commit_phase2(phase1_output, prover_id, sector_id)
    }
}

pub fn seal_pre_commit_phase1(
    registered_proof: RegisteredSealProof,
    cache_path: PathBuf,
    in_path: PathBuf,
    out_path: PathBuf,
    prover_id: ProverId,
    sector_id: SectorId,
    ticket: Ticket,
    piece_infos: &[PieceInfo],
) -> Result<SealPreCommitPhase1Output> {
    safe_call! {
        seal::seal_pre_commit_phase1(
            registered_proof,
            cache_path,
            in_path,
            out_path,
            prover_id,
            sector_id,
            ticket,
            piece_infos,
        )
    }
}

pub fn seal_pre_commit_phase2(
    phase1_output: SealPreCommitPhase1Output,
    cache_path: PathBuf,
    out_path: PathBuf,
) -> Result<SealPreCommitPhase2Output> {
    safe_call! {
        seal::seal_pre_commit_phase2(phase1_output, cache_path, out_path)
    }
}
