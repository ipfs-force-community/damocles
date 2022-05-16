//! resource control, acquiring & releasing

use std::collections::HashMap;

use anyhow::Result;
use crossbeam_channel::{bounded, Receiver, Sender};

use crate::logging::debug;

const LOG_TARGET: &str = "resource";

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
    fn new(size: usize) -> Self {
        let (tx, rx) = bounded(size);

        Limit { tx, rx }
    }

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
        let mut pool = HashMap::new();

        for (k, v) in iter {
            debug!(target: LOG_TARGET, name = k.as_str(), limit = *v, "add limitation");
            pool.insert(k.to_owned(), Limit::new(*v));
        }

        Pool { pool }
    }

    /// acquires a token for the named resource
    pub fn acquire<N: AsRef<str>>(&self, name: N) -> Result<Option<Token>> {
        let key = name.as_ref();
        match self.pool.get(key) {
            Some(limit) => limit.acquire().map(|t| {
                debug!(target: LOG_TARGET, name = key, "acquired");
                Some(t)
            }),

            None => {
                debug!(target: LOG_TARGET, name = key, "unlimited");
                Ok(None)
            }
        }
    }
}
