//! resource control, acquiring & releasing

use std::collections::HashMap;

use anyhow::{anyhow, Result};
use crossbeam_channel::{Receiver, Sender};

type LimitTx = Sender<()>;
type LimitRx = Receiver<()>;

/// Token represents 1 available resource unit
pub struct Token(LimitRx);

impl Drop for Token {
    fn drop(&mut self) {
        let _ = self.0.recv();
    }
}

struct Limit {
    tx: LimitTx,
    rx: LimitRx,
}

impl Limit {
    fn acquire(&self) -> Result<Token> {
        self.tx.send(())?;

        Ok(Token(self.rx.clone()))
    }
}

/// resource limit pool
pub struct Pool {
    pool: HashMap<String, Limit>,
}

impl Pool {
    /// construct a pool with given name-size mapping
    pub fn new<'a, I: Iterator<Item = (&'a String, &'a usize)>>(iter: I) -> Self {
        unimplemented!();
    }

    /// acquires a token for the named resource
    pub fn acquire<N: AsRef<str>>(&self, name: N) -> Result<Token> {
        let key = name.as_ref();
        let limit = self
            .pool
            .get(name.as_ref())
            .ok_or(anyhow!("resource limit for {} is unavailable", key))?;

        limit.acquire()
    }
}
