//! helper utilities for sealing

use anyhow::{anyhow, Result};

use crate::sealing::processor::Commitment;

const ALL_ZERO_PIECE_COMMS: [Commitment; 35] = [
    [
        0x37, 0x31, 0xbb, 0x99, 0xac, 0x68, 0x9f, 0x66, 0xee, 0xf5, 0x97, 0x3e, 0x4a, 0x94, 0xda, 0x18, 0x8f, 0x4d, 0xdc, 0xae, 0x58, 0x7,
        0x24, 0xfc, 0x6f, 0x3f, 0xd6, 0xd, 0xfd, 0x48, 0x83, 0x33,
    ],
    [
        0x64, 0x2a, 0x60, 0x7e, 0xf8, 0x86, 0xb0, 0x4, 0xbf, 0x2c, 0x19, 0x78, 0x46, 0x3a, 0xe1, 0xd4, 0x69, 0x3a, 0xc0, 0xf4, 0x10, 0xeb,
        0x2d, 0x1b, 0x7a, 0x47, 0xfe, 0x20, 0x5e, 0x5e, 0x75, 0xf,
    ],
    [
        0x57, 0xa2, 0x38, 0x1a, 0x28, 0x65, 0x2b, 0xf4, 0x7f, 0x6b, 0xef, 0x7a, 0xca, 0x67, 0x9b, 0xe4, 0xae, 0xde, 0x58, 0x71, 0xab, 0x5c,
        0xf3, 0xeb, 0x2c, 0x8, 0x11, 0x44, 0x88, 0xcb, 0x85, 0x26,
    ],
    [
        0x1f, 0x7a, 0xc9, 0x59, 0x55, 0x10, 0xe0, 0x9e, 0xa4, 0x1c, 0x46, 0xb, 0x17, 0x64, 0x30, 0xbb, 0x32, 0x2c, 0xd6, 0xfb, 0x41, 0x2e,
        0xc5, 0x7c, 0xb1, 0x7d, 0x98, 0x9a, 0x43, 0x10, 0x37, 0x2f,
    ],
    [
        0xfc, 0x7e, 0x92, 0x82, 0x96, 0xe5, 0x16, 0xfa, 0xad, 0xe9, 0x86, 0xb2, 0x8f, 0x92, 0xd4, 0x4a, 0x4f, 0x24, 0xb9, 0x35, 0x48, 0x52,
        0x23, 0x37, 0x6a, 0x79, 0x90, 0x27, 0xbc, 0x18, 0xf8, 0x33,
    ],
    [
        0x8, 0xc4, 0x7b, 0x38, 0xee, 0x13, 0xbc, 0x43, 0xf4, 0x1b, 0x91, 0x5c, 0xe, 0xed, 0x99, 0x11, 0xa2, 0x60, 0x86, 0xb3, 0xed, 0x62,
        0x40, 0x1b, 0xf9, 0xd5, 0x8b, 0x8d, 0x19, 0xdf, 0xf6, 0x24,
    ],
    [
        0xb2, 0xe4, 0x7b, 0xfb, 0x11, 0xfa, 0xcd, 0x94, 0x1f, 0x62, 0xaf, 0x5c, 0x75, 0xf, 0x3e, 0xa5, 0xcc, 0x4d, 0xf5, 0x17, 0xd5, 0xc4,
        0xf1, 0x6d, 0xb2, 0xb4, 0xd7, 0x7b, 0xae, 0xc1, 0xa3, 0x2f,
    ],
    [
        0xf9, 0x22, 0x61, 0x60, 0xc8, 0xf9, 0x27, 0xbf, 0xdc, 0xc4, 0x18, 0xcd, 0xf2, 0x3, 0x49, 0x31, 0x46, 0x0, 0x8e, 0xae, 0xfb, 0x7d,
        0x2, 0x19, 0x4d, 0x5e, 0x54, 0x81, 0x89, 0x0, 0x51, 0x8,
    ],
    [
        0x2c, 0x1a, 0x96, 0x4b, 0xb9, 0xb, 0x59, 0xeb, 0xfe, 0xf, 0x6d, 0xa2, 0x9a, 0xd6, 0x5a, 0xe3, 0xe4, 0x17, 0x72, 0x4a, 0x8f, 0x7c,
        0x11, 0x74, 0x5a, 0x40, 0xca, 0xc1, 0xe5, 0xe7, 0x40, 0x11,
    ],
    [
        0xfe, 0xe3, 0x78, 0xce, 0xf1, 0x64, 0x4, 0xb1, 0x99, 0xed, 0xe0, 0xb1, 0x3e, 0x11, 0xb6, 0x24, 0xff, 0x9d, 0x78, 0x4f, 0xbb, 0xed,
        0x87, 0x8d, 0x83, 0x29, 0x7e, 0x79, 0x5e, 0x2, 0x4f, 0x2,
    ],
    [
        0x8e, 0x9e, 0x24, 0x3, 0xfa, 0x88, 0x4c, 0xf6, 0x23, 0x7f, 0x60, 0xdf, 0x25, 0xf8, 0x3e, 0xe4, 0xd, 0xca, 0x9e, 0xd8, 0x79, 0xeb,
        0x6f, 0x63, 0x52, 0xd1, 0x50, 0x84, 0xf5, 0xad, 0xd, 0x3f,
    ],
    [
        0x75, 0x2d, 0x96, 0x93, 0xfa, 0x16, 0x75, 0x24, 0x39, 0x54, 0x76, 0xe3, 0x17, 0xa9, 0x85, 0x80, 0xf0, 0x9, 0x47, 0xaf, 0xb7, 0xa3,
        0x5, 0x40, 0xd6, 0x25, 0xa9, 0x29, 0x1c, 0xc1, 0x2a, 0x7,
    ],
    [
        0x70, 0x22, 0xf6, 0xf, 0x7e, 0xf6, 0xad, 0xfa, 0x17, 0x11, 0x7a, 0x52, 0x61, 0x9e, 0x30, 0xce, 0xa8, 0x2c, 0x68, 0x7, 0x5a, 0xdf,
        0x1c, 0x66, 0x77, 0x86, 0xec, 0x50, 0x6e, 0xef, 0x2d, 0x19,
    ],
    [
        0xd9, 0x98, 0x87, 0xb9, 0x73, 0x57, 0x3a, 0x96, 0xe1, 0x13, 0x93, 0x64, 0x52, 0x36, 0xc1, 0x7b, 0x1f, 0x4c, 0x70, 0x34, 0xd7, 0x23,
        0xc7, 0xa9, 0x9f, 0x70, 0x9b, 0xb4, 0xda, 0x61, 0x16, 0x2b,
    ],
    [
        0xd0, 0xb5, 0x30, 0xdb, 0xb0, 0xb4, 0xf2, 0x5c, 0x5d, 0x2f, 0x2a, 0x28, 0xdf, 0xee, 0x80, 0x8b, 0x53, 0x41, 0x2a, 0x2, 0x93, 0x1f,
        0x18, 0xc4, 0x99, 0xf5, 0xa2, 0x54, 0x8, 0x6b, 0x13, 0x26,
    ],
    [
        0x84, 0xc0, 0x42, 0x1b, 0xa0, 0x68, 0x5a, 0x1, 0xbf, 0x79, 0x5a, 0x23, 0x44, 0x6, 0x4f, 0xe4, 0x24, 0xbd, 0x52, 0xa9, 0xd2, 0x43,
        0x77, 0xb3, 0x94, 0xff, 0x4c, 0x4b, 0x45, 0x68, 0xe8, 0x11,
    ],
    [
        0x65, 0xf2, 0x9e, 0x5d, 0x98, 0xd2, 0x46, 0xc3, 0x8b, 0x38, 0x8c, 0xfc, 0x6, 0xdb, 0x1f, 0x6b, 0x2, 0x13, 0x3, 0xc5, 0xa2, 0x89,
        0x0, 0xb, 0xdc, 0xe8, 0x32, 0xa9, 0xc3, 0xec, 0x42, 0x1c,
    ],
    [
        0xa2, 0x24, 0x75, 0x8, 0x28, 0x58, 0x50, 0x96, 0x5b, 0x7e, 0x33, 0x4b, 0x31, 0x27, 0xb0, 0xc0, 0x42, 0xb1, 0xd0, 0x46, 0xdc, 0x54,
        0x40, 0x21, 0x37, 0x62, 0x7c, 0xd8, 0x79, 0x9c, 0xe1, 0x3a,
    ],
    [
        0xda, 0xfd, 0xab, 0x6d, 0xa9, 0x36, 0x44, 0x53, 0xc2, 0x6d, 0x33, 0x72, 0x6b, 0x9f, 0xef, 0xe3, 0x43, 0xbe, 0x8f, 0x81, 0x64, 0x9e,
        0xc0, 0x9, 0xaa, 0xd3, 0xfa, 0xff, 0x50, 0x61, 0x75, 0x8,
    ],
    [
        0xd9, 0x41, 0xd5, 0xe0, 0xd6, 0x31, 0x4a, 0x99, 0x5c, 0x33, 0xff, 0xbd, 0x4f, 0xbe, 0x69, 0x11, 0x8d, 0x73, 0xd4, 0xe5, 0xfd, 0x2c,
        0xd3, 0x1f, 0xf, 0x7c, 0x86, 0xeb, 0xdd, 0x14, 0xe7, 0x6,
    ],
    [
        0x51, 0x4c, 0x43, 0x5c, 0x3d, 0x4, 0xd3, 0x49, 0xa5, 0x36, 0x5f, 0xbd, 0x59, 0xff, 0xc7, 0x13, 0x62, 0x91, 0x11, 0x78, 0x59, 0x91,
        0xc1, 0xa3, 0xc5, 0x3a, 0xf2, 0x20, 0x79, 0x74, 0x1a, 0x2f,
    ],
    [
        0xad, 0x6, 0x85, 0x39, 0x69, 0xd3, 0x7d, 0x34, 0xff, 0x8, 0xe0, 0x9f, 0x56, 0x93, 0xa, 0x4a, 0xd1, 0x9a, 0x89, 0xde, 0xf6, 0xc,
        0xbf, 0xee, 0x7e, 0x1d, 0x33, 0x81, 0xc1, 0xe7, 0x1c, 0x37,
    ],
    [
        0x39, 0x56, 0xe, 0x7b, 0x13, 0xa9, 0x3b, 0x7, 0xa2, 0x43, 0xfd, 0x27, 0x20, 0xff, 0xa7, 0xcb, 0x3e, 0x1d, 0x2e, 0x50, 0x5a, 0xb3,
        0x62, 0x9e, 0x79, 0xf4, 0x63, 0x13, 0x51, 0x2c, 0xda, 0x6,
    ],
    [
        0xcc, 0xc3, 0xc0, 0x12, 0xf5, 0xb0, 0x5e, 0x81, 0x1a, 0x2b, 0xbf, 0xdd, 0xf, 0x68, 0x33, 0xb8, 0x42, 0x75, 0xb4, 0x7b, 0xf2, 0x29,
        0xc0, 0x5, 0x2a, 0x82, 0x48, 0x4f, 0x3c, 0x1a, 0x5b, 0x3d,
    ],
    [
        0x7d, 0xf2, 0x9b, 0x69, 0x77, 0x31, 0x99, 0xe8, 0xf2, 0xb4, 0xb, 0x77, 0x91, 0x9d, 0x4, 0x85, 0x9, 0xee, 0xd7, 0x68, 0xe2, 0xc7,
        0x29, 0x7b, 0x1f, 0x14, 0x37, 0x3, 0x4f, 0xc3, 0xc6, 0x2c,
    ],
    [
        0x66, 0xce, 0x5, 0xa3, 0x66, 0x75, 0x52, 0xcf, 0x45, 0xc0, 0x2b, 0xcc, 0x4e, 0x83, 0x92, 0x91, 0x9b, 0xde, 0xac, 0x35, 0xde, 0x2f,
        0xf5, 0x62, 0x71, 0x84, 0x8e, 0x9f, 0x7b, 0x67, 0x51, 0x7,
    ],
    [
        0xd8, 0x61, 0x2, 0x18, 0x42, 0x5a, 0xb5, 0xe9, 0x5b, 0x1c, 0xa6, 0x23, 0x9d, 0x29, 0xa2, 0xe4, 0x20, 0xd7, 0x6, 0xa9, 0x6f, 0x37,
        0x3e, 0x2f, 0x9c, 0x9a, 0x91, 0xd7, 0x59, 0xd1, 0x9b, 0x1,
    ],
    [
        0x6d, 0x36, 0x4b, 0x1e, 0xf8, 0x46, 0x44, 0x1a, 0x5a, 0x4a, 0x68, 0x86, 0x23, 0x14, 0xac, 0xc0, 0xa4, 0x6f, 0x1, 0x67, 0x17, 0xe5,
        0x34, 0x43, 0xe8, 0x39, 0xee, 0xdf, 0x83, 0xc2, 0x85, 0x3c,
    ],
    [
        0x7, 0x7e, 0x5f, 0xde, 0x35, 0xc5, 0xa, 0x93, 0x3, 0xa5, 0x50, 0x9, 0xe3, 0x49, 0x8a, 0x4e, 0xbe, 0xdf, 0xf3, 0x9c, 0x42, 0xb7,
        0x10, 0xb7, 0x30, 0xd8, 0xec, 0x7a, 0xc7, 0xaf, 0xa6, 0x3e,
    ],
    [
        0xe6, 0x40, 0x5, 0xa6, 0xbf, 0xe3, 0x77, 0x79, 0x53, 0xb8, 0xad, 0x6e, 0xf9, 0x3f, 0xf, 0xca, 0x10, 0x49, 0xb2, 0x4, 0x16, 0x54,
        0xf2, 0xa4, 0x11, 0xf7, 0x70, 0x27, 0x99, 0xce, 0xce, 0x2,
    ],
    [
        0x25, 0x9d, 0x3d, 0x6b, 0x1f, 0x4d, 0x87, 0x6d, 0x11, 0x85, 0xe1, 0x12, 0x3a, 0xf6, 0xf5, 0x50, 0x1a, 0xf0, 0xf6, 0x7c, 0xf1, 0x5b,
        0x52, 0x16, 0x25, 0x5b, 0x7b, 0x17, 0x8d, 0x12, 0x5, 0x1d,
    ],
    [
        0x3f, 0x9a, 0x4d, 0x41, 0x1d, 0xa4, 0xef, 0x1b, 0x36, 0xf3, 0x5f, 0xf0, 0xa1, 0x95, 0xae, 0x39, 0x2a, 0xb2, 0x3f, 0xee, 0x79, 0x67,
        0xb7, 0xc4, 0x1b, 0x3, 0xd1, 0x61, 0x3f, 0xc2, 0x92, 0x39,
    ],
    [
        0xfe, 0x4e, 0xf3, 0x28, 0xc6, 0x1a, 0xa3, 0x9c, 0xfd, 0xb2, 0x48, 0x4e, 0xaa, 0x32, 0xa1, 0x51, 0xb1, 0xfe, 0x3d, 0xfd, 0x1f, 0x96,
        0xdd, 0x8c, 0x97, 0x11, 0xfd, 0x86, 0xd6, 0xc5, 0x81, 0x13,
    ],
    [
        0xf5, 0x5d, 0x68, 0x90, 0xe, 0x2d, 0x83, 0x81, 0xec, 0xcb, 0x81, 0x64, 0xcb, 0x99, 0x76, 0xf2, 0x4b, 0x2d, 0xe0, 0xdd, 0x61, 0xa3,
        0x1b, 0x97, 0xce, 0x6e, 0xb2, 0x38, 0x50, 0xd5, 0xe8, 0x19,
    ],
    [
        0xaa, 0xaa, 0x8c, 0x4c, 0xb4, 0xa, 0xac, 0xee, 0x1e, 0x2, 0xdc, 0x65, 0x42, 0x4b, 0x2a, 0x6c, 0x8e, 0x99, 0xf8, 0x3, 0xb7, 0x2f,
        0x79, 0x29, 0xc4, 0x10, 0x1d, 0x7f, 0xae, 0x6b, 0xff, 0x32,
    ],
];

#[inline]
fn sector_size_to_commitment_index(sector_size: u64) -> Result<usize> {
    let idx = sector_size.trailing_zeros() as usize - 2 - 5;
    if idx >= 35 {
        return Err(anyhow!("sector size {} too large"));
    }

    Ok(idx)
}

/// gets commitment for all-zero piece of sector size
pub fn get_all_zero_commitment(sector_size: u64) -> Result<Commitment> {
    let idx = sector_size_to_commitment_index(sector_size)?;

    Ok(ALL_ZERO_PIECE_COMMS[idx])
}

/// returns if the commitment is a all-zero piece
pub fn is_all_zero_commitment(sector_size: u64, comm: Commitment) -> Result<bool> {
    let idx = sector_size_to_commitment_index(sector_size)?;

    Ok(comm == ALL_ZERO_PIECE_COMMS[idx])
}
