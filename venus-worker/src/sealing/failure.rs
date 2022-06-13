use std::error::Error as StdError;
use std::fmt::Display;

use anyhow::Error;

#[derive(Debug, Eq, PartialEq)]
pub enum Level {
    // tempoary error, may be retried after a while
    Temporary,

    // failed for a couple of times after retrying
    Permanent,

    // the whole sealing task should be abortted
    Abort,

    // the worker should be paused
    Critical,
}

pub struct Failure(pub Level, pub Error);

impl std::fmt::Debug for Failure {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.0 {
            Level::Temporary => f.write_str(&format!("temporary: {:?}", self.1)),
            Level::Permanent => f.write_str(&format!("permanent: {:?}", self.1)),
            Level::Abort => f.write_str(&format!("abort: {:?}", self.1)),
            Level::Critical => f.write_str(&format!("critical: {:?}", self.1)),
        }
    }
}

impl std::fmt::Display for Failure {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.0 {
            Level::Temporary => f.write_str(&format!("temporary: {}", self.1)),
            Level::Permanent => f.write_str(&format!("permanent: {}", self.1)),
            Level::Abort => f.write_str(&format!("abort: {}", self.1)),
            Level::Critical => f.write_str(&format!("critical: {}", self.1)),
        }
    }
}

pub trait FailureContext<T> {
    fn context<C>(self, c: C) -> Result<T, Failure>
    where
        C: Display + Send + Sync + 'static;

    fn with_context<C, F>(self, f: F) -> Result<T, Failure>
    where
        C: Display + Send + Sync + 'static,
        F: FnOnce() -> C;
}

impl<T> FailureContext<T> for Result<T, Failure> {
    fn context<C>(self, c: C) -> Result<T, Failure>
    where
        C: Display + Send + Sync + 'static,
    {
        self.map_err(|e| Failure(e.0, e.1.context(c)))
    }

    fn with_context<C, F>(self, f: F) -> Result<T, Failure>
    where
        C: Display + Send + Sync + 'static,
        F: FnOnce() -> C,
    {
        self.map_err(|e| Failure(e.0, e.1.context(f())))
    }
}

pub trait IntoFailure {
    fn temp(self) -> Failure;

    fn perm(self) -> Failure;

    fn abort(self) -> Failure;

    fn crit(self) -> Failure;
}

impl IntoFailure for Error {
    fn temp(self) -> Failure {
        Failure(Level::Temporary, self)
    }

    fn perm(self) -> Failure {
        Failure(Level::Permanent, self)
    }

    fn abort(self) -> Failure {
        Failure(Level::Abort, self)
    }

    fn crit(self) -> Failure {
        Failure(Level::Critical, self)
    }
}

pub trait StdIntoFailure {
    fn temp(self) -> Failure;

    fn perm(self) -> Failure;

    fn abort(self) -> Failure;

    fn crit(self) -> Failure;
}

impl<E> StdIntoFailure for E
where
    E: StdError + Send + Sync + 'static,
{
    fn temp(self) -> Failure {
        Error::new(self).temp()
    }

    fn perm(self) -> Failure {
        Error::new(self).perm()
    }

    fn abort(self) -> Failure {
        Error::new(self).abort()
    }

    fn crit(self) -> Failure {
        Error::new(self).crit()
    }
}

pub trait MapErrToFailure<T> {
    fn temp(self) -> Result<T, Failure>;

    fn perm(self) -> Result<T, Failure>;

    fn abort(self) -> Result<T, Failure>;

    fn crit(self) -> Result<T, Failure>;
}

impl<T> MapErrToFailure<T> for Result<T, Error> {
    fn temp(self) -> Result<T, Failure> {
        self.map_err(|e| e.temp())
    }

    fn perm(self) -> Result<T, Failure> {
        self.map_err(|e| e.perm())
    }

    fn abort(self) -> Result<T, Failure> {
        self.map_err(|e| e.abort())
    }

    fn crit(self) -> Result<T, Failure> {
        self.map_err(|e| e.crit())
    }
}

pub trait MapStdErrToFailure<T> {
    fn temp(self) -> Result<T, Failure>;

    fn perm(self) -> Result<T, Failure>;

    fn abort(self) -> Result<T, Failure>;

    fn crit(self) -> Result<T, Failure>;
}

impl<T, E> MapStdErrToFailure<T> for Result<T, E>
where
    E: StdError + Send + Sync + 'static,
{
    fn temp(self) -> Result<T, Failure> {
        self.map_err(|e| Error::new(e).temp())
    }

    fn perm(self) -> Result<T, Failure> {
        self.map_err(|e| Error::new(e).perm())
    }

    fn abort(self) -> Result<T, Failure> {
        self.map_err(|e| Error::new(e).abort())
    }

    fn crit(self) -> Result<T, Failure> {
        self.map_err(|e| Error::new(e).crit())
    }
}

#[derive(Debug, Clone, Copy)]
pub struct Interrupt;

impl std::fmt::Display for Interrupt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("interrupt")
    }
}

impl StdError for Interrupt {}

impl From<Interrupt> for Failure {
    fn from(int: Interrupt) -> Self {
        Failure(Level::Permanent, int.into())
    }
}
