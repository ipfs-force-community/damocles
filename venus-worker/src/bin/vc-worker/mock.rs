use std::sync::Arc;
use std::thread;

use anyhow::{anyhow, Result};
use async_std::task;
use crossbeam_channel::{bounded, select};
use fil_types::ActorID;
use jsonrpc_core::IoHandler;
use jsonrpc_core_client::transports::local;

use venus_worker::{
    infra::objstore::filestore::FileStore,
    logging::{debug_field, error, info, warn},
    rpc::{self, SealerRpc, SealerRpcClient},
    sealing::{config, resource, store::StoreManager, util::size2proof},
};

pub fn start_mock(miner: ActorID, sector_size: u64, cfg_path: String) -> Result<()> {
    let proof_type = size2proof(sector_size)?;

    info!(
        miner,
        sector_size,
        proof_type = debug_field(proof_type),
        config = cfg_path.as_str(),
        "start initializing mock impl"
    );

    let cfg = config::Config::load(&cfg_path)?;

    let remote_store = cfg
        .remote
        .path
        .ok_or(anyhow!("remote path is required for mock"))?;
    let remote = Arc::new(FileStore::open(remote_store)?);

    let mock_impl = rpc::mock::SimpleMockSealerRpc::new(miner, proof_type);
    let mut io = IoHandler::new();
    io.extend_with(mock_impl.to_delegate());

    let (done_tx, done_rx) = bounded(0);

    let (mock_client, mock_server) = local::connect::<SealerRpcClient, _, _>(io);
    let (server_stop_tx, server_stop_rx) = bounded::<()>(0);

    thread::spawn(move || {
        info!("mock server start");
        match task::block_on(mock_server) {
            Ok(()) => {
                info!("mock server stop");
            }

            Err(e) => {
                error!(err = debug_field(&e), "mock server stop");
            }
        };

        drop(server_stop_tx);
    });

    let (mgr_stop_tx, mgr_stop_rx) = bounded::<()>(0);

    let limit = resource::Pool::new(cfg.limit.iter());

    let store_mgr = StoreManager::load(&cfg.store, &cfg.sealing)?;
    thread::spawn(move || {
        info!("store mgr start");
        store_mgr.start_sealing(done_rx, Arc::new(mock_client), remote, Arc::new(limit));
        drop(mgr_stop_tx);
    });

    select! {
        recv(server_stop_rx) -> _srv_stop => {
            warn!("mock server stopped");
        },

        recv(mgr_stop_rx) -> _mgr_stop => {
            warn!("store mgr stopped");
        }
    }

    drop(done_tx);

    Ok(())
}
