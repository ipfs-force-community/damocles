use std::convert::TryFrom;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use fil_types::ActorID;
use jsonrpc_core::IoHandler;
use jsonrpc_core_client::transports::{local, ws};

use crate::{
    config,
    infra::objstore::filestore::FileStore,
    logging::{debug_field, info},
    rpc::sealer::{mock, Sealer, SealerClient},
    sealing::{resource, seal, service, store::StoreManager},
    signal::Signal,
    types::SealProof,
    watchdog::{dones, GlobalModules, WatchDog},
};

/// start a worker process with mock modules
pub fn start_mock(miner: ActorID, sector_size: u64, cfg_path: String) -> Result<()> {
    let proof_type = SealProof::try_from(sector_size)?;

    info!(
        miner,
        sector_size,
        proof_type = debug_field(proof_type),
        config = cfg_path.as_str(),
        "start initializing mock impl"
    );

    let cfg = config::Config::load(&cfg_path)?;

    info!("config loaded:\n {:?}", cfg);

    let remote = cfg
        .remote
        .path
        .as_ref()
        .cloned()
        .ok_or(anyhow!("remote path is required for mock"))?;
    let remote_store = Box::new(FileStore::open(remote)?);

    let mock_impl = mock::SimpleMockSealerRpc::new(miner, proof_type);
    let mut io = IoHandler::new();
    io.extend_with(mock_impl.to_delegate());

    let (pc2, pc2sub): (seal::BoxedPC2Processor, Option<_>) = if let Some(ext) = cfg
        .processors
        .as_ref()
        .and_then(|p| p.pc2.as_ref())
        .and_then(|ext| if ext.external { Some(ext) } else { None })
    {
        let (proc, sub) = seal::external::PC2::build(ext)?;
        (Box::new(proc), Some(sub))
    } else {
        (Box::new(seal::internal::PC2), None)
    };

    let (c2, c2sub): (seal::BoxedC2Processor, Option<_>) = if let Some(ext) = cfg
        .processors
        .as_ref()
        .and_then(|p| p.c2.as_ref())
        .and_then(|ext| if ext.external { Some(ext) } else { None })
    {
        let (proc, sub) = seal::external::C2::build(ext)?;
        (Box::new(proc), Some(sub))
    } else {
        (Box::new(seal::internal::C2), None)
    };

    let done = dones();

    let mock_client = local::connect::<SealerClient, _, _>(done.1.clone(), io)
        .map_err(|e| anyhow!("build local client: {:?}", e))?;

    let store_mgr = StoreManager::load(&cfg.store, &cfg.sealing)?;
    let workers = store_mgr.into_workers();

    let globl = GlobalModules {
        rpc: Arc::new(mock_client),
        remote_store: Arc::new(remote_store),
        pc2: Arc::new(pc2),
        c2: Arc::new(c2),
        limit: Arc::new(resource::Pool::new(cfg.limit.iter())),
    };

    let mut dog = WatchDog::build_with_done(cfg, globl, done);

    let mut ctrls = Vec::new();
    for (worker, ctrl) in workers {
        ctrls.push(ctrl);
        dog.start_module(worker);
    }

    let worker_server = service::Service::new(ctrls);
    dog.start_module(worker_server);

    if let Some(sub) = pc2sub {
        dog.start_module(sub);
    };

    if let Some(sub) = c2sub {
        dog.start_module(sub)
    }

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    Ok(())
}

/// start a normal venus-worker daemon
pub fn start_deamon(cfg_path: String) -> Result<()> {
    let cfg = config::Config::load(&cfg_path)?;
    info!("config loaded\n {:?}", cfg);

    let remote_store = cfg
        .remote
        .path
        .as_ref()
        .cloned()
        .ok_or(anyhow!("remote path is required for mock"))?;
    let remote = Box::new(FileStore::open(remote_store)?);

    let store_mgr = StoreManager::load(&cfg.store, &cfg.sealing)?;

    let done = dones();

    let rpc_connect_req = ws::ConnectInfo {
        url: format!("{}{}", cfg.sealer_rpc.endpoint, "/rpc/v0"),
        headers: Default::default(),
    };

    let rpc_client =
        ws::connect(done.1.clone(), rpc_connect_req).map_err(|e| anyhow!("ws connect: {:?}", e))?;

    let (pc2, pc2sub): (seal::BoxedPC2Processor, Option<_>) = if let Some(ext) = cfg
        .processors
        .as_ref()
        .and_then(|p| p.pc2.as_ref())
        .and_then(|ext| if ext.external { Some(ext) } else { None })
    {
        let (proc, sub) = seal::external::PC2::build(ext)?;
        (Box::new(proc), Some(sub))
    } else {
        (Box::new(seal::internal::PC2), None)
    };

    let (c2, c2sub): (seal::BoxedC2Processor, Option<_>) = if let Some(ext) = cfg
        .processors
        .as_ref()
        .and_then(|p| p.c2.as_ref())
        .and_then(|ext| if ext.external { Some(ext) } else { None })
    {
        let (proc, sub) = seal::external::C2::build(ext)?;
        (Box::new(proc), Some(sub))
    } else {
        (Box::new(seal::internal::C2), None)
    };

    let workers = store_mgr.into_workers();

    let globl = GlobalModules {
        rpc: Arc::new(rpc_client),
        remote_store: Arc::new(remote),
        pc2: Arc::new(pc2),
        c2: Arc::new(c2),
        limit: Arc::new(resource::Pool::new(cfg.limit.iter())),
    };

    let mut dog = WatchDog::build(cfg, globl);

    let mut ctrls = Vec::new();
    for (worker, ctrl) in workers {
        ctrls.push(ctrl);
        dog.start_module(worker);
    }

    let worker_server = service::Service::new(ctrls);
    dog.start_module(worker_server);

    if let Some(sub) = pc2sub {
        dog.start_module(sub);
    };

    if let Some(sub) = c2sub {
        dog.start_module(sub)
    }

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    Ok(())
}
