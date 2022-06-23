use anyhow::{anyhow, Error, Result};
use base64::STANDARD;
use base64_serde::base64_serde_type;
use serde::{Deserialize, Serialize};

base64_serde_type!(pub B64SerDe, STANDARD);

/// Vec<u8> with base64 ser & de
#[derive(Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(transparent)]
pub struct BytesVec(#[serde(with = "B64SerDe")] pub Vec<u8>);

impl From<Vec<u8>> for BytesVec {
    fn from(val: Vec<u8>) -> BytesVec {
        BytesVec(val)
    }
}

impl From<&Vec<u8>> for BytesVec {
    fn from(val: &Vec<u8>) -> BytesVec {
        BytesVec(val.clone())
    }
}

/// [u8; 32]  with base64 ser & de
#[derive(Copy, Clone, Debug, Default, PartialEq, Hash, Eq, Serialize, Deserialize)]
#[serde(into = "BytesVec")]
#[serde(try_from = "BytesVec")]
pub struct BytesArray32(pub [u8; 32]);

impl From<[u8; 32]> for BytesArray32 {
    fn from(val: [u8; 32]) -> Self {
        BytesArray32(val)
    }
}

impl TryFrom<BytesVec> for BytesArray32 {
    type Error = Error;

    fn try_from(v: BytesVec) -> Result<Self> {
        if v.0.len() != 32 {
            return Err(anyhow!("expected 32 bytes, got {}", v.0.len()));
        }

        let mut a = [0u8; 32];
        a.copy_from_slice(&v.0[..]);

        Ok(BytesArray32(a))
    }
}

impl From<BytesArray32> for BytesVec {
    fn from(r: BytesArray32) -> Self {
        let mut v = vec![0; 32];
        v.copy_from_slice(&r.0[..]);
        BytesVec(v)
    }
}
