use std::io::{repeat, Error, ErrorKind, Read, Result};

use fil_types::UnpaddedPieceSize;

pub fn inflator<R: Read>(
    inner: R,
    payload_size: u64,
    target_size: UnpaddedPieceSize,
) -> Result<impl Read> {
    target_size
        .validate()
        .map_err(|e| Error::new(ErrorKind::InvalidInput, e))?;

    if payload_size > target_size.0 {
        return Err(Error::new(
            ErrorKind::InvalidInput,
            "payload size larger than target size",
        ));
    }

    Ok(inner
        .take(payload_size)
        .chain(repeat(0).take(target_size.0 - payload_size)))
}
