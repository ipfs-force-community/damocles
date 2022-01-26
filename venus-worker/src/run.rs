use std::collections::HashMap;
use std::convert::TryFrom;
use std::path::PathBuf;
use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use fil_types::ActorID;
use jsonrpc_core::IoHandler;
use jsonrpc_core_client::transports::{local, ws};

use crate::{
    config,
    infra::objstore::filestore::FileStore,
    logging::{debug_field, info},
    rpc::sealer::{mock, Sealer, SealerClient},
    sealing::{processor, resource, service, store::StoreManager},
    signal::Signal,
    types::SealProof,
    util::net::{local_interface_ip, socket_addr_from_url},
    watchdog::{GloablProcessors, GlobalModules, Module, WatchDog},
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

    let cfg = config::Config::load(&cfg_path)
        .with_context(|| format!("load from config file {}", cfg_path))?;

    info!("config loaded:\n {:?}", cfg);

    let remote = cfg
        .remote
        .path
        .as_ref()
        .cloned()
        .ok_or(anyhow!("remote path is required for mock"))?;
    let remote_store = Box::new(
        FileStore::open(&remote, cfg.remote.instance.clone())
            .with_context(|| format!("open remote filestore {}", remote))?,
    );

    let mock_impl = mock::SimpleMockSealerRpc::new(miner, proof_type);
    let mut io = IoHandler::new();
    io.extend_with(mock_impl.to_delegate());

    let mock_client = local::connect::<SealerClient, _, _>(io)
        .map_err(|e| anyhow!("build local client: {:?}", e))?;

    let (processors, modules) = start_processors(&cfg).context("start processors")?;

    let store_mgr = StoreManager::load(&cfg.store, &cfg.sealing).context("load store manager")?;
    let workers = store_mgr.into_workers();

    let static_tree_d = construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let globl = GlobalModules {
        rpc: Arc::new(mock_client),
        remote_store: Arc::new(remote_store),
        processors,
        limit: Arc::new(resource::Pool::new(cfg.limit.iter())),
        static_tree_d,
    };

    let instance = cfg
        .instance
        .as_ref()
        .and_then(|s| s.name.as_ref())
        .cloned()
        .unwrap_or("mock".to_owned());

    let mut dog = WatchDog::build(cfg, instance, globl);

    let mut ctrls = Vec::new();
    for (worker, ctrl) in workers {
        ctrls.push(ctrl);
        dog.start_module(worker);
    }

    let worker_server = service::Service::new(ctrls);
    dog.start_module(worker_server);

    for m in modules {
        dog.start_module(m);
    }

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    Ok(())
}

/// start a normal venus-worker daemon
pub fn start_deamon(cfg_path: String) -> Result<()> {
    let cfg = config::Config::load(&cfg_path)
        .with_context(|| format!("load from config file {}", cfg_path))?;
    info!("config loaded\n {:?}", cfg);

    let remote_store = cfg
        .remote
        .path
        .as_ref()
        .cloned()
        .ok_or(anyhow!("remote path is required for deamon"))?;
    let remote = Box::new(
        FileStore::open(&remote_store, cfg.remote.instance.clone())
            .with_context(|| format!("open remote filestore {}", remote_store))?,
    );

    let store_mgr = StoreManager::load(&cfg.store, &cfg.sealing).context("load store manager")?;

    let rpc_connect_req = cfg.sealer_rpc.to_connect_info();

    let rpc_client = ws::connect(rpc_connect_req)
        .map_err(|e| anyhow!("ws connect: {:?}", e))
        .with_context(|| format!("rpc url {}", cfg.sealer_rpc.url))?;

    let instance = if let Some(name) = cfg.instance.as_ref().and_then(|s| s.name.as_ref()).cloned()
    {
        name
    } else {
        let local_ip = socket_addr_from_url(&cfg.sealer_rpc.url)
            .with_context(|| {
                format!(
                    "attempt to connect to sealer rpc service {}",
                    &cfg.sealer_rpc.url
                )
            })
            .and_then(local_interface_ip)
            .context("get local ip")?;
        format!("{}", local_ip)
    };

    let (processors, modules) = start_processors(&cfg).context("start processors")?;

    let workers = store_mgr.into_workers();

    let static_tree_d = construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let globl = GlobalModules {
        rpc: Arc::new(rpc_client),
        remote_store: Arc::new(remote),
        processors,
        limit: Arc::new(resource::Pool::new(cfg.limit.iter())),
        static_tree_d,
    };

    let mut dog = WatchDog::build(cfg, instance, globl);

    let mut ctrls = Vec::new();
    for (worker, ctrl) in workers {
        ctrls.push(ctrl);
        dog.start_module(worker);
    }

    let worker_server = service::Service::new(ctrls);
    dog.start_module(worker_server);

    for m in modules {
        dog.start_module(m);
    }

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    Ok(())
}

fn construct_static_tree_d(cfg: &config::Config) -> Result<HashMap<u64, PathBuf>> {
    let mut trees = HashMap::new();
    if let Some(c) = cfg.static_tree_d.as_ref() {
        for (k, v) in c {
            let b = Byte::from_str(k).with_context(|| format!("invalid bytes string {}", k))?;
            let size = b.get_bytes() as u64;
            SealProof::try_from(size).with_context(|| format!("invalid sector size {}", k))?;
            let tree_path = PathBuf::from(v.to_owned())
                .canonicalize()
                .with_context(|| format!("invalid tree_d path {} for sector size {}", v, k))?;

            trees.insert(size, tree_path);
        }
    }

    Ok(trees)
}

macro_rules! construct_sub_processor {
    ($field:ident, $cfg:ident, $modules:ident) => {
        if let Some(ext) = $cfg
            .processors
            .as_ref()
            .and_then(|p| p.$field.as_ref())
            .and_then(|ext| if ext.external { Some(ext) } else { None })
        {
            let (proc, subs) = processor::external::ExtProcessor::build(ext)?;
            for sub in subs {
                $modules.push(Box::new(sub));
            }

            Box::new(proc)
        } else {
            Box::new(processor::internal::Proc::new())
        }
    };
}

fn start_processors(cfg: &config::Config) -> Result<(GloablProcessors, Vec<Box<dyn Module>>)> {
    let mut modules: Vec<Box<dyn Module>> = Vec::new();

    let tree_d: processor::BoxedTreeDProcessor = construct_sub_processor!(tree_d, cfg, modules);

    let pc1: processor::BoxedPC1Processor = construct_sub_processor!(pc1, cfg, modules);

    let pc2: processor::BoxedPC2Processor = construct_sub_processor!(pc2, cfg, modules);

    let c2: processor::BoxedC2Processor = construct_sub_processor!(c2, cfg, modules);

    Ok((
        GloablProcessors {
            tree_d: Arc::new(tree_d),
            pc1: Arc::new(pc1),
            pc2: Arc::new(pc2),
            c2: Arc::new(c2),
        },
        modules,
    ))
}
