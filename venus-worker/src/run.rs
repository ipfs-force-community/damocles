use std::collections::HashMap;
use std::convert::TryFrom;
use std::env;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use jsonrpc_core_client::transports::http;
use metrics_exporter_prometheus::PrometheusBuilder;
use reqwest::Url;
use tokio::{runtime::Builder, sync::Semaphore};
use tracing::{info, warn};

use crate::{
    config,
    infra::{
        objstore::{attached::AttachedManager, filestore::FileStore, ObjectStore},
        piecestore::{local::LocalPieceStore, remote::RemotePieceStore, ComposePieceStore, EmptyPieceStore, PieceStore},
    },
    rpc::sealer::SealerClient,
    sealing::{ping, processor, service, store::StoreManager, GlobalProcessors},
    signal::Signal,
    types::SealProof,
    util::net::{local_interface_ip, rpc_addr, socket_addr_from_url},
    watchdog::{GlobalModules, WatchDog},
};

/// start a normal venus-worker daemon
pub fn start_daemon(cfg_path: String) -> Result<()> {
    let runtime = Builder::new_multi_thread().enable_all().build().context("construct runtime")?;

    let mut cfg = config::Config::load(&cfg_path).with_context(|| format!("load from config file {}", cfg_path))?;
    match cfg.render() {
        Ok(s) => info!("config loaded\n {}", s),
        Err(e) => warn!(err=?e, "unable to render config"),
    }

    compatible_for_piece_token(&mut cfg);

    let dial_addr = rpc_addr(&cfg.sector_manager.rpc_client.addr, 0)?;
    info!(
        raw = %cfg.sector_manager.rpc_client.addr,
        addr = %dial_addr.as_str(),
        "rpc dial info"
    );

    let rpc_client: SealerClient = runtime
        .block_on(async { http::connect(&dial_addr).await })
        .map_err(|e| anyhow!("jsonrpc connect to {}: {:?}", &dial_addr, e))?;

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

    // check all persist store exist in venus-sector-manager
    for st in attached.iter() {
        let ins_name = st.instance();
        if runtime
            .block_on(async { rpc_client.store_basic_info(ins_name).await })
            .map_err(|e| anyhow!("rpc error: {:?}", e))
            .with_context(|| format!("request for store basic info of instance {}", st.instance()))?
            .is_none()
        {
            return Err(anyhow!(
                "store basic info of instance {} not found in venus-sector-manager",
                st.instance()
            ));
        }
    }

    info!("{} stores attached, {} writable", attached.len(), attached_writable);

    let attached_mgr = AttachedManager::init(attached).context("init attached manager")?;

    let store_mgr = StoreManager::load(&cfg.sealing_thread, &cfg.sealing).context("load store manager")?;

    let local_ip = socket_addr_from_url(&dial_addr)
        .with_context(|| format!("attempt to connect to sealer rpc service {}", &dial_addr))
        .and_then(local_interface_ip)
        .context("get local ip")?;

    let instance = if let Some(name) = cfg.worker.as_ref().and_then(|s| s.name.as_ref()).cloned() {
        name
    } else {
        format!("{}", local_ip)
    };

    if cfg.metrics.enable {
        let mut builder = PrometheusBuilder::new()
            .add_global_label("worker_name", instance.clone())
            .add_global_label("worker_ip", local_ip.to_string());

        if let Some(listen) = cfg.metrics.http_listen.as_ref().cloned() {
            builder = builder.with_http_listener(listen);
        }

        builder.install().context("install prometheus recorder")?;
        info!("prometheus exproter inited");
    }

    let dest = format!("{}:{}", local_ip, cfg.worker_server_listen_port());
    info!(?instance, ?dest, "worker info inited");

    let rpc_origin = Url::parse(&dial_addr)
        .map(|u| u.origin().ascii_serialization())
        .context("parse rpc url origin")?;

    let piece_store: Arc<dyn PieceStore> = if cfg.sealing.enable_deals.unwrap_or(false) {
        let create_remote_piece_store = || RemotePieceStore::new(&rpc_origin).context("build proxy piece store");
        if let Some(local_pieces_dir) = cfg.worker_local_pieces_dir() {
            Arc::new(ComposePieceStore::new(
                LocalPieceStore::new(local_pieces_dir),
                create_remote_piece_store()?,
            ))
        } else {
            Arc::new(create_remote_piece_store()?)
        }
    } else {
        Arc::new(EmptyPieceStore)
    };

    let ext_locks: HashMap<String, Arc<Semaphore>> = cfg
        .processors
        .ext_locks
        .clone().unwrap_or_default()
        .into_iter()
        .map(|(name, permits)| (name, Arc::new(Semaphore::new(permits))))
        .collect();

    let processors = start_processors(&cfg, &cfg.processors.limitation, &ext_locks).context("start processors")?;

    let workers = store_mgr.into_workers(processors);

    let static_tree_d = construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let rt = Arc::new(runtime);
    let global = GlobalModules {
        rpc: Arc::new(rpc_client),
        attached: Arc::new(attached_mgr),
        static_tree_d,
        rt: rt.clone(),
        piece_store,
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

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    if let Ok(rt) = Arc::try_unwrap(rt) {
        rt.shutdown_timeout(Duration::from_secs(5));
    }

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

fn start_processors(cfg: &config::Config, limit: &config::Limit, ext_locks: &HashMap<String, Arc<Semaphore>>) -> Result<GlobalProcessors> {
    use processor::*;

    let add_pieces = create_add_pieces_processor(
        cfg.processors.add_pieces(),
        limit.delay(STAGE_NAME_ADD_PIECES),
        limit.concurrent(STAGE_NAME_ADD_PIECES),
        ext_locks,
    )?;
    let tree_d = create_tree_d_processor(
        cfg.processors.tree_d(),
        limit.delay(STAGE_NAME_TREED),
        limit.concurrent(STAGE_NAME_TREED),
        ext_locks,
    )?;
    let pc1 = create_p_c1_processor(
        cfg.processors.pc1(),
        limit.delay(STAGE_NAME_PC1),
        limit.concurrent(STAGE_NAME_PC1),
        ext_locks,
    )?;
    let pc2 = create_p_c2_processor(
        cfg.processors.pc2(),
        limit.delay(STAGE_NAME_PC2),
        limit.concurrent(STAGE_NAME_PC2),
        ext_locks,
    )?;
    let c2 = create_c2_processor(
        cfg.processors.c2(),
        limit.delay(STAGE_NAME_C2),
        limit.concurrent(STAGE_NAME_C2),
        ext_locks,
    )?;
    let snap_encode = create_snap_encode_processor(
        cfg.processors.snap_encode(),
        limit.delay(STAGE_NAME_SNAP_ENCODE),
        limit.concurrent(STAGE_NAME_SNAP_ENCODE),
        ext_locks,
    )?;
    let snap_prove = create_snap_prove_processor(
        cfg.processors.snap_prove(),
        limit.delay(STAGE_NAME_SNAP_PROVE),
        limit.concurrent(STAGE_NAME_SNAP_PROVE),
        ext_locks,
    )?;
    let transfer = create_transfer_processor(
        cfg.processors.transfer(),
        limit.delay(STAGE_NAME_TRANSFER),
        limit.concurrent(STAGE_NAME_TRANSFER),
        ext_locks,
    )?;

    Ok(GlobalProcessors {
        add_pieces,
        tree_d,
        pc1,
        pc2,
        c2,
        snap_encode,
        snap_prove,
        transfer,
    })
}

fn compatible_for_piece_token(cfg: &mut config::Config) {
    use vc_fil_consumers::builtin::executors::piece::fetcher::http::PieceHttpFetcher;

    if let Some(token) = &cfg.sector_manager.piece_token {
        match &mut cfg.processors.add_pieces {
            Some(add_piece_cfgs) => add_piece_cfgs.iter_mut().for_each(|cfg| {
                cfg.envs
                    .get_or_insert_with(|| HashMap::with_capacity(1))
                    .insert(PieceHttpFetcher::ENV_KEY_PIECE_FETCHER_TOKEN.to_string(), token.clone());
            }),
            None => env::set_var(PieceHttpFetcher::ENV_KEY_PIECE_FETCHER_TOKEN, token),
        }
    }
}
