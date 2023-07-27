macro_rules! call_rpc {
    (raw, $client:expr=>$method:ident($($arg:expr,)*)) => {
        {
            crate::metrics::rpc::VIEW.call.method(stringify!($method)).incr();
            let now = std::time::Instant::now();
            let res = crate::block_on($client.$method($($arg,)*));

            crate::metrics::rpc::VIEW.timing.method(stringify!($method)).record(now.elapsed());

            if res.is_err() {
                crate::metrics::rpc::VIEW.error.method(stringify!($method)).incr();
            }

            res
        }
    };
    ($client:expr=>$method:ident($($arg:expr,)*)) => {
        {
            call_rpc!(raw, $client=>$method($($arg,)*)).map_err(|e| {
                if let jsonrpc_core_client::RpcError::JsonRpcError(ref je) = e {
                    if je.code == jsonrpc_core::types::error::ErrorCode::ServerError(crate::rpc::APIErrCode::SectorStateNotFound as i64) {
                        return anyhow::anyhow!("from error code: sector state not found, with msg: {}", je.message).abort()
                    }
                }

                anyhow::anyhow!("rpc error: {}", e).temp()
            })
        }
    };
}

pub(super) use call_rpc;

macro_rules! field_required {
    ($name:ident, $ex:expr) => {
        let $name = $ex.with_context(|| format!("{} is required", stringify!(name))).abort()?;
    };
}

pub(super) use field_required;

macro_rules! cloned_required {
    ($name:ident, $ex:expr) => {
        let $name = $ex
            .as_ref()
            .cloned()
            .with_context(|| format!("{} is required", stringify!(name)))
            .abort()?;
    };
}

pub(super) use cloned_required;
