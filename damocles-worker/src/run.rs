use std::convert::TryFrom;
use std::env;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use std::{collections::HashMap, path::Path};

use anyhow::{anyhow, Context, Result};
use byte_unit::Byte;
use jsonrpc_core_client::transports::http;
use metrics_exporter_prometheus::PrometheusBuilder;
use reqwest::Url;
use tokio::runtime::Builder;
use vc_processors::builtin::processors::BuiltinProcessor;

use crate::sealing::build_sealing_threads;
use crate::{
    config,
    infra::{
        objstore::{attached::AttachedManager, filestore::FileStore, ObjectStore},
        piecestore::{local::LocalPieceStore, remote::RemotePieceStore, ComposePieceStore, EmptyPieceStore, PieceStore},
    },
    logging::{info, warn},
    rpc::sealer::SealerClient,
    sealing::{
        ping, processor,
        resource::{self, LimitItem},
        service,
    },
    signal::Signal,
    types::SealProof,
    util::net::{local_interface_ip, rpc_addr},
    watchdog::{GlobalModules, GlobalProcessors, WatchDog},
};

/// start a normal damocles-worker daemon
pub fn start_daemon(cfg_path: impl AsRef<Path>) -> Result<()> {
    let runtime = Builder::new_multi_thread().enable_all().build().context("construct runtime")?;

    let mut cfg = config::Config::load(&cfg_path).with_context(|| format!("load from config file {}", cfg_path.as_ref().display()))?;
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

    // check all persist store exist in damocles-manager
    for st in attached.iter() {
        let ins_name = st.instance();
        if runtime
            .block_on(async { rpc_client.store_basic_info(ins_name).await })
            .map_err(|e| anyhow!("rpc error: {:?}", e))
            .with_context(|| format!("request for store basic info of instance {}", st.instance()))?
            .is_none()
        {
            return Err(anyhow!(
                "store basic info of instance {} not found in damocles-manager",
                st.instance()
            ));
        }
    }

    info!("{} stores attached, {} writable", attached.len(), attached_writable);

    let attached_mgr = AttachedManager::init(attached).context("init attached manager")?;

    let sealing_threads = build_sealing_threads(&cfg.sealing_thread, &cfg.sealing).context("build sealing thread")?;

    let socket_addrs = Url::parse(&dial_addr)
        .with_context(|| format!("invalid url: {}", dial_addr))?
        .socket_addrs(|| None)
        .with_context(|| format!("attempt to resolve a url's host and port: {}", dial_addr))?;
    let local_ip = local_interface_ip(socket_addrs.as_slice()).context("get local ip")?;

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

    let remote_piece_store = RemotePieceStore::new(&rpc_origin).context("build proxy piece store")?;
    let piece_store: Arc<dyn PieceStore> = if cfg.sealing.enable_deals.unwrap_or(false) {
        let local_pieces_dirs = cfg.worker_local_pieces_dirs();
        if !local_pieces_dirs.is_empty() {
            Arc::new(ComposePieceStore::new(
                LocalPieceStore::new(local_pieces_dirs),
                remote_piece_store.clone(),
            ))
        } else {
            Arc::new(remote_piece_store.clone())
        }
    } else {
        Arc::new(EmptyPieceStore)
    };

    let remote_piece_store = Arc::new(remote_piece_store);

    let ext_locks = Arc::new(create_resource_pool(&cfg.processors.ext_locks, &None));

    let processors = start_processors(&cfg, &ext_locks).context("start processors")?;

    let static_tree_d = construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let rt = Arc::new(runtime);
    let global = GlobalModules {
        rpc: Arc::new(rpc_client),
        attached: Arc::new(attached_mgr),
        processors,
        limit: Arc::new(create_resource_pool(
            cfg.processors.limitation_concurrent(),
            &cfg.processors.limitation.staggered,
        )),
        ext_locks,
        static_tree_d,
        rt: rt.clone(),
        piece_store,
        remote_piece_store,
    };

    let worker_ping_interval = cfg.worker_ping_interval();

    let mut dog = WatchDog::build(cfg, instance, dest, global);

    let mut ctrls = Vec::new();
    for (sealing_thread, ctrl) in sealing_threads {
        ctrls.push(ctrl);
        dog.start_module(sealing_thread);
    }

    let sealing_thread_ctrls = Arc::new(ctrls);

    let worker_server = service::Service::new(sealing_thread_ctrls.clone());
    dog.start_module(worker_server);

    let worker_ping = ping::Ping::new(worker_ping_interval, sealing_thread_ctrls);
    dog.start_module(worker_ping);

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    if let Ok(rt) = Arc::try_unwrap(rt) {
        rt.shutdown_timeout(Duration::from_secs(5));
    }

    Ok(())
}

fn create_resource_pool(
    concurrent_limit_opt: &Option<HashMap<String, usize>>,
    staggered_limit_opt: &Option<HashMap<String, config::SerdeDuration>>,
) -> resource::Pool {
    resource::Pool::new(merge_limit_config(concurrent_limit_opt, staggered_limit_opt))
}

#[inline]
fn merge_limit_config<'a>(
    concurrent_limit_opt: &'a Option<HashMap<String, usize>>,
    staggered_limit_opt: &'a Option<HashMap<String, config::SerdeDuration>>,
) -> impl Iterator<Item = LimitItem<'a>> {
    let concurrent_map_len = concurrent_limit_opt.as_ref().map(|x| x.len()).unwrap_or(0);
    let staggered_map_len = staggered_limit_opt.as_ref().map(|x| x.len()).unwrap_or(0);
    let mut limits: HashMap<&str, LimitItem<'_>> = HashMap::with_capacity(concurrent_map_len.max(staggered_map_len));

    if let Some(concurrent_limit) = concurrent_limit_opt {
        for (name, concurrent) in concurrent_limit {
            limits.insert(
                name,
                LimitItem {
                    name,
                    concurrent: Some(concurrent),
                    staggered_interval: None,
                },
            );
        }
    }

    if let Some(staggered_limit) = staggered_limit_opt {
        for (name, interval) in staggered_limit {
            limits
                .entry(name)
                .and_modify(|limit_item| limit_item.staggered_interval = Some(&interval.0))
                .or_insert_with(|| LimitItem {
                    name,
                    concurrent: None,
                    staggered_interval: Some(&interval.0),
                });
        }
    }
    limits.into_values()
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
    ($field:ident, $cfg:ident, $locks:ident) => {
        if let Some(ext) = $cfg.processors.$field.as_ref() {
            let proc = processor::external::ExtProcessor::build(ext, $locks.clone())?;
            Arc::new(proc)
        } else {
            Arc::new(BuiltinProcessor::default())
        }
    };
}

fn start_processors(cfg: &config::Config, locks: &Arc<resource::Pool>) -> Result<GlobalProcessors> {
    let add_pieces: processor::ArcAddPiecesProcessor = construct_sub_processor!(add_pieces, cfg, locks);

    let tree_d: processor::ArcTreeDProcessor = construct_sub_processor!(tree_d, cfg, locks);

    let pc1: processor::ArcPC1Processor = construct_sub_processor!(pc1, cfg, locks);

    let pc2: processor::ArcPC2Processor = construct_sub_processor!(pc2, cfg, locks);

    let c2: processor::ArcC2Processor = construct_sub_processor!(c2, cfg, locks);

    let snap_encode: processor::ArcSnapEncodeProcessor = construct_sub_processor!(snap_encode, cfg, locks);

    let snap_prove: processor::ArcSnapProveProcessor = construct_sub_processor!(snap_prove, cfg, locks);

    let transfer: processor::ArcTransferProcessor = construct_sub_processor!(transfer, cfg, locks);

    let unseal: processor::ArcUnsealProcessor = construct_sub_processor!(unseal, cfg, locks);

    let wdpost: processor::ArcWdPostProcessor = construct_sub_processor!(wdpost, cfg, locks);

    Ok(GlobalProcessors {
        add_pieces,
        tree_d,
        pc1,
        pc2,
        c2,
        snap_encode,
        snap_prove,
        transfer,
        unseal,
        wdpost,
    })
}

fn compatible_for_piece_token(cfg: &mut config::Config) {
    use vc_processors::builtin::processors::piece::fetcher::http::PieceHttpFetcher;

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
#[cfg(test)]
mod tests {
    use crate::{config::SerdeDuration, sealing::resource::LimitItem};

    use super::merge_limit_config;
    use humantime::parse_duration;
    use pretty_assertions::assert_eq;

    #[test]
    fn test_merge_limit_config() {
        let cases = vec![
            (
                Some(vec![("pc1", 10), ("pc2", 20)]),
                Some(vec![("pc1", "1s"), ("pc2", "2s")]),
                vec![
                    ("pc1", Some(10), Some(parse_duration("1s").unwrap())),
                    ("pc2", Some(20), Some(parse_duration("2s").unwrap())),
                ],
            ),
            (
                Some(vec![("pc2", 20)]),
                Some(vec![("pc1", "1s"), ("pc2", "2s")]),
                vec![
                    ("pc1", None, Some(parse_duration("1s").unwrap())),
                    ("pc2", Some(20), Some(parse_duration("2s").unwrap())),
                ],
            ),
            (
                Some(vec![("pc2", 20)]),
                Some(vec![("pc1", "1s")]),
                vec![("pc1", None, Some(parse_duration("1s").unwrap())), ("pc2", Some(20), None)],
            ),
            (
                None,
                Some(vec![("pc1", "1s"), ("pc2", "2s")]),
                vec![
                    ("pc1", None, Some(parse_duration("1s").unwrap())),
                    ("pc2", None, Some(parse_duration("2s").unwrap())),
                ],
            ),
            (
                Some(vec![("pc1", 10), ("pc2", 20)]),
                None,
                vec![("pc1", Some(10), None), ("pc2", Some(20), None)],
            ),
            (None, None, vec![]),
        ];

        for (concurrent_limit, staggered_limit, result) in cases {
            let concurrent_limit_map_opt = concurrent_limit.map(|x| x.into_iter().map(|(name, x)| (name.to_string(), x)).collect());
            let staggered_limit_map_opt = staggered_limit.map(|x| {
                x.into_iter()
                    .map(|(name, dur)| (name.to_string(), SerdeDuration(parse_duration(dur).unwrap())))
                    .collect()
            });
            let merged = merge_limit_config(&concurrent_limit_map_opt, &staggered_limit_map_opt);

            let mut expect = result
                .iter()
                .map(|x| LimitItem {
                    name: x.0,
                    concurrent: x.1.as_ref(),
                    staggered_interval: x.2.as_ref(),
                })
                .collect::<Vec<_>>();
            let mut actual = merged.collect::<Vec<_>>();

            expect.sort_by(|x, y| x.name.cmp(y.name));
            actual.sort_by(|x, y| x.name.cmp(y.name));

            assert_eq!(
                format!("{:?}", actual),
                format!("{:?}", expect),
                "testing concurrent_limit: {:?}, staggered_limit: {:?}",
                concurrent_limit_map_opt,
                staggered_limit_map_opt
            );
        }
    }
}
