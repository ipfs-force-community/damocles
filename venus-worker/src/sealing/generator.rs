use std::convert::TryFrom;
use std::fs::{write, OpenOptions};
use std::io::{self, prelude::*};

use anyhow::Result;

use crate::sealing::processor::{write_and_preprocess, PaddedBytesAmount, UnpaddedBytesAmount};

use crate::types::SealProof;

/// generate defalt piece for cc sector
pub fn generate_piece(sector_size: u64, staged_path: String, pieces_path: String) -> Result<()> {
    let proof_type = SealProof::try_from(sector_size)?;

    let unpadded_size: UnpaddedBytesAmount = PaddedBytesAmount(sector_size).into();

    let mut pledge_piece = io::repeat(0).take(unpadded_size.0);

    let mut staged_file = OpenOptions::new()
        .create(true)
        .read(true)
        .write(true)
        // to make sure that we won't write into the staged file with any data exists
        .truncate(true)
        .open(&staged_path)?;

    let (piece_info, _) = write_and_preprocess(
        proof_type.into(),
        &mut pledge_piece,
        &mut staged_file,
        unpadded_size,
    )
    .unwrap();

    let mut pieces = Vec::new();
    pieces.push(piece_info);

    let str_piece = serde_json::to_string(&pieces).unwrap();
    write(&pieces_path, str_piece).unwrap();

    Ok(())
}
