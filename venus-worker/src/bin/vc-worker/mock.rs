use std::thread;

use anyhow::{anyhow, Result};
use async_std::task;
use crossbeam_channel::bounded;
use fil_types::ActorID;
use filecoin_proofs_api::RegisteredSealProof;
use jsonrpc_core::IoHandler;
use jsonrpc_core_client::transports::local;

use venus_worker::{
    logging::{debug_field, error, info},
    rpc::{self, SealerRpc, SealerRpcClient},
};

const SIZE_2K: u64 = 2 << 10;
const SIZE_8M: u64 = 8 << 20;
const SIZE_512M: u64 = 512 << 20;
const SIZE_32G: u64 = 32 << 30;
const SIZE_64G: u64 = 64 << 30;

pub fn start_mock(miner: ActorID, sector_size: u64) -> Result<()> {
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
        "init mock impl"
    );

    let mock_impl = rpc::mock::SimpleMockSealerRpc::new(miner, proof_type);
    let mut io = IoHandler::new();
    io.extend_with(mock_impl.to_delegate());

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

    let _ = server_stop_rx.recv();
    Ok(())
}
