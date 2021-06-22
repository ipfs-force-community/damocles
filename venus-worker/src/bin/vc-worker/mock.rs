use std::sync::Arc;
use std::thread;

use anyhow::{anyhow, Result};
use async_std::task;
use crossbeam_channel::{bounded, select};
use fil_types::ActorID;
use filecoin_proofs_api::RegisteredSealProof;
use jsonrpc_core::IoHandler;
use jsonrpc_core_client::transports::local;

use venus_worker::{
    infra::objstore::filestore::FileStore,
    logging::{debug_field, error, info, warn},
    rpc::{self, SealerRpc, SealerRpcClient},
    sealing::store::{util::load_store_list, StoreManager},
};

const SIZE_2K: u64 = 2 << 10;
const SIZE_8M: u64 = 8 << 20;
const SIZE_512M: u64 = 512 << 20;
const SIZE_32G: u64 = 32 << 30;
const SIZE_64G: u64 = 64 << 30;

pub fn start_mock(
    miner: ActorID,
    sector_size: u64,
    store_list: String,
    remote_store: String,
) -> Result<()> {
    let remote = Arc::new(FileStore::open(remote_store)?);

    let store_paths = load_store_list(store_list)?;

    let proof_type = match sector_size {
        SIZE_2K => RegisteredSealProof::StackedDrg2KiBV1_1,
        SIZE_8M => RegisteredSealProof::StackedDrg8MiBV1_1,
        SIZE_512M => RegisteredSealProof::StackedDrg512MiBV1_1,
        SIZE_32G => RegisteredSealProof::StackedDrg32GiBV1_1,
        SIZE_64G => RegisteredSealProof::StackedDrg64GiBV1_1,
        other => return Err(anyhow!("invalid sector size {}", other)),
    };

    info!(
        miner,
        sector_size,
        proof_type = debug_field(proof_type),
        stores = debug_field(&store_paths),
        "init mock impl"
    );

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

    let store_mgr = StoreManager::load(store_paths)?;
    thread::spawn(move || {
        info!("store mgr start");
        store_mgr.start_sealing(done_rx, Arc::new(mock_client), remote);
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
