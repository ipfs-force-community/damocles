//! rpc definitions & client implementations

pub mod sealer;
pub mod worker;

#[derive(Debug)]
#[repr(i64)]
pub enum APIErrCode {
    SectorStateNotFound = 11001,
}
