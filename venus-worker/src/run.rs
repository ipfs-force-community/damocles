use std::collections::HashMap;
use std::convert::TryFrom;
use std::path::PathBuf;
use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use jsonrpc_core_client::transports::http;
use reqwest::Url;
use tokio::runtime::Builder;

use crate::{
    config,
    infra::{
        objstore::{attached::AttachedManager, filestore::FileStore, ObjectStore},
        piecestore::{proxy::ProxyPieceStore, PieceStore},
    },
    logging::info,
    sealing::{ping, processor, resource, service, store::StoreManager},
    signal::Signal,
    types::SealProof,
    util::net::{local_interface_ip, rpc_addr, socket_addr_from_url},
    watchdog::{GloablProcessors, GlobalModules, Module, WatchDog},
};

/// start a normal venus-worker daemon
pub fn start_deamon(cfg_path: String) -> Result<()> {
    let runtime = Builder::new_multi_thread().enable_all().build().context("construct runtime")?;

    let cfg = config::Config::load(&cfg_path).with_context(|| format!("load from config file {}", cfg_path))?;
    info!("config loaded\n {:?}", cfg);

    let mut attached: Vec<Box<dyn ObjectStore>> = Vec::new();
    let mut attached_writable = 0;
    if let Some(remote_cfg) = cfg.remote_store.as_ref() {
        let remote_store = Box::new(
            FileStore::open(
                remote_cfg.location.clone(),
                remote_cfg.name.clone(),
                remote_cfg.readonly.unwrap_or(false),
            )
            .with_context(|| format!("open remote filestore {}", remote_cfg.location))?,
        );

        if !remote_store.readonly() {
            attached_writable += 1;
        }

        attached.push(remote_store);
    }

    if let Some(attach_cfgs) = cfg.attached.as_ref() {
        for (sidx, scfg) in attach_cfgs.iter().enumerate() {
            let attached_store = Box::new(
                FileStore::open(scfg.location.clone(), scfg.name.clone(), scfg.readonly.unwrap_or(false))
                    .with_context(|| format!("open attached filestore #{}", sidx))?,
            );

            if !attached_store.readonly() {
                attached_writable += 1;
            }

            attached.push(attached_store);
        }
    }

    if attached.is_empty() {
        return Err(anyhow!("no attached store available"));
    }

    if attached_writable == 0 {
        return Err(anyhow!("no attached store available for writing"));
    }

    info!("{} stores attached, {} writable", attached.len(), attached_writable);

    let attached_mgr = AttachedManager::init(attached).context("init attached manager")?;

    let store_mgr = StoreManager::load(&cfg.sealing_thread, &cfg.sealing).context("load store manager")?;

    let dial_addr = rpc_addr(&cfg.sector_manager.rpc_client.addr, 0)?;
    info!(
        raw = %cfg.sector_manager.rpc_client.addr,
        addr = %dial_addr.as_str(),
        "rpc dial info"
    );

    let rpc_client = runtime
        .block_on(async { http::connect(&dial_addr).await })
        .map_err(|e| anyhow!("jsonrpc connect to {}: {:?}", &dial_addr, e))?;

    let local_ip = socket_addr_from_url(&dial_addr)
        .with_context(|| format!("attempt to connect to sealer rpc service {}", &dial_addr))
        .and_then(local_interface_ip)
        .context("get local ip")?;

    let instance = if let Some(name) = cfg.worker.as_ref().and_then(|s| s.name.as_ref()).cloned() {
        name
    } else {
        format!("{}", local_ip)
    };

    let dest = format!("{}:{}", local_ip, cfg.worker_server_listen_port());
    info!(?instance, ?dest, "worker info inited");

    let rpc_origin = Url::parse(&dial_addr)
        .map(|u| u.origin().ascii_serialization())
        .context("parse rpc url origin")?;

    let piece_store: Option<Box<dyn PieceStore>> = if cfg.sealing.enable_deals.unwrap_or(false) {
        Some(Box::new(
            ProxyPieceStore::new(&rpc_origin, cfg.sector_manager.piece_token.as_ref().cloned()).context("build proxy piece store")?,
        ))
    } else {
        None
    };

    let ext_locks = Arc::new(resource::Pool::new(
        cfg.processors.ext_locks.as_ref().cloned().unwrap_or_default().iter(),
    ));

    let (processors, modules) = start_processors(&cfg, &ext_locks).context("start processors")?;

    let workers = store_mgr.into_workers();

    let static_tree_d = construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let global = GlobalModules {
        rpc: Arc::new(rpc_client),
        attached: Arc::new(attached_mgr),
        processors,
        limit: Arc::new(resource::Pool::new(
            cfg.processors.limit.as_ref().cloned().unwrap_or_default().iter(),
        )),
        ext_locks,
        static_tree_d,
        rt: Arc::new(runtime),
        piece_store: piece_store.map(Arc::new),
    };

    let worker_ping_interval = cfg.worker_ping_interval();

    let mut dog = WatchDog::build(cfg, instance, dest, global);

    let mut ctrls = Vec::new();
    for (worker, ctrl) in workers {
        ctrls.push(ctrl);
        dog.start_module(worker);
    }

    let worker_ctrls = Arc::new(ctrls);

    let worker_server = service::Service::new(worker_ctrls.clone());
    dog.start_module(worker_server);

    let worker_ping = ping::Ping::new(worker_ping_interval, worker_ctrls);
    dog.start_module(worker_ping);

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
    if let Some(c) = cfg.processors.static_tree_d.as_ref() {
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
    ($field:ident, $cfg:ident, $locks:ident, $modules:ident) => {
        if let Some(ext) = $cfg.processors.$field.as_ref() {
            let (proc, subs) = processor::external::ExtProcessor::build(ext, $locks.clone())?;
            for sub in subs {
                $modules.push(Box::new(sub));
            }

            Box::new(proc)
        } else {
            Box::new(processor::internal::Proc::new())
        }
    };
}

fn start_processors(cfg: &config::Config, locks: &Arc<resource::Pool>) -> Result<(GloablProcessors, Vec<Box<dyn Module>>)> {
    let mut modules: Vec<Box<dyn Module>> = Vec::new();

    let tree_d: processor::BoxedTreeDProcessor = construct_sub_processor!(tree_d, cfg, locks, modules);

    let pc1: processor::BoxedPC1Processor = construct_sub_processor!(pc1, cfg, locks, modules);

    let pc2: processor::BoxedPC2Processor = construct_sub_processor!(pc2, cfg, locks, modules);

    let c2: processor::BoxedC2Processor = construct_sub_processor!(c2, cfg, locks, modules);

    let snap_encode: processor::BoxedSnapEncodeProcessor = construct_sub_processor!(snap_encode, cfg, locks, modules);

    let snap_prove: processor::BoxedSnapProveProcessor = construct_sub_processor!(snap_prove, cfg, locks, modules);

    Ok((
        GloablProcessors {
            tree_d: Arc::new(tree_d),
            pc1: Arc::new(pc1),
            pc2: Arc::new(pc2),
            c2: Arc::new(c2),
            snap_encode: Arc::new(snap_encode),
            snap_prove: Arc::new(snap_prove),
        },
        modules,
    ))
}
