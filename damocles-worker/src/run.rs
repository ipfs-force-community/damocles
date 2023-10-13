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

use crate::limit::{SealingLimit, SealingLimitBuilder};
use crate::sealing::build_sealing_threads;
use crate::sealing::processor::{Either, NoLockProcessor};
use crate::{
    config,
    infra::{
        objstore::{
            attached::AttachedManager, filestore::FileStore, ObjectStore,
        },
        piecestore::{
            local::LocalPieceStore, remote::RemotePieceStore,
            ComposePieceStore, EmptyPieceStore, PieceStore,
        },
    },
    logging::{info, warn},
    rpc::sealer::SealerClient,
    sealing::{ping, processor, resource::LimitItem, service},
    signal::Signal,
    types::seal_types_from_u64,
    util::net::{local_interface_ip, rpc_addr},
    watchdog::{CtrlProc, GlobalModules, GlobalProcessors, WatchDog},
};

/// start a normal damocles-worker daemon
pub fn start_daemon(cfg_path: impl AsRef<Path>) -> Result<()> {
    let runtime = Builder::new_multi_thread()
        .enable_all()
        .build()
        .context("construct runtime")?;

    let mut cfg = config::Config::load(&cfg_path).with_context(|| {
        format!("load from config file {}", cfg_path.as_ref().display())
    })?;
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
            .with_context(|| {
                format!("open remote filestore {}", remote_cfg.location)
            })?,
        );

        if !remote_store.readonly() {
            attached_writable += 1;
        }

        attached.push(remote_store);
    }

    if let Some(attach_cfgs) = cfg.attached.as_ref() {
        for (sidx, scfg) in attach_cfgs.iter().enumerate() {
            let attached_store = Box::new(
                FileStore::open(
                    scfg.location.clone(),
                    scfg.name.clone(),
                    scfg.readonly.unwrap_or(false),
                )
                .with_context(|| {
                    format!("open attached filestore #{}", sidx)
                })?,
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
            .with_context(|| {
                format!(
                    "request for store basic info of instance {}",
                    st.instance()
                )
            })?
            .is_none()
        {
            return Err(anyhow!(
                "store basic info of instance {} not found in damocles-manager",
                st.instance()
            ));
        }
    }

    info!(
        "{} stores attached, {} writable",
        attached.len(),
        attached_writable
    );

    let attached_mgr =
        AttachedManager::init(attached).context("init attached manager")?;

    let limit = Arc::new(build_limit(&cfg));

    let sealing_threads =
        build_sealing_threads(&cfg.sealing_thread, &cfg.sealing, limit.clone())
            .context("build sealing thread")?;

    let socket_addrs = Url::parse(&dial_addr)
        .with_context(|| format!("invalid url: {}", dial_addr))?
        .socket_addrs(|| None)
        .with_context(|| {
            format!("attempt to resolve a url's host and port: {}", dial_addr)
        })?;
    let local_ip =
        local_interface_ip(socket_addrs.as_slice()).context("get local ip")?;

    let instance = if let Some(name) =
        cfg.worker.as_ref().and_then(|s| s.name.as_ref()).cloned()
    {
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

    let remote_piece_store = RemotePieceStore::new(&rpc_origin)
        .context("build proxy piece store")?;
    let piece_store: Arc<dyn PieceStore> =
        if cfg.sealing.enable_deals.unwrap_or(false) {
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

    let processors =
        start_processors(&cfg, limit).context("start processors")?;
    let static_tree_d =
        construct_static_tree_d(&cfg).context("check static tree-d files")?;

    let rt = Arc::new(runtime);
    let global = GlobalModules {
        rpc: Arc::new(rpc_client),
        attached: Arc::new(attached_mgr),
        processors: Arc::new(processors),
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

    let worker_ping =
        ping::Ping::new(worker_ping_interval, sealing_thread_ctrls);
    dog.start_module(worker_ping);

    dog.start_module(Signal);

    // TODO: handle result
    let _ = dog.wait();

    if let Ok(rt) = Arc::try_unwrap(rt) {
        rt.shutdown_timeout(Duration::from_secs(5));
    }

    Ok(())
}

fn build_limit(cfg: &config::Config) -> SealingLimit {
    let mut limit_builder = SealingLimitBuilder::new();
    if let Some(ext_locks) = &cfg.processors.ext_locks {
        limit_builder.extend_ext_locks_limit(ext_locks.clone());
    }
    limit_builder.extend_stage_limits(merge_limit_config(
        cfg.processors.limitation_concurrent().clone(),
        cfg.processors.limitation.staggered.clone(),
    ));

    limit_builder.build()
}

#[inline]
fn merge_limit_config(
    concurrent_limit_opt: Option<HashMap<String, usize>>,
    staggered_limit_opt: Option<HashMap<String, config::SerdeDuration>>,
) -> impl Iterator<Item = LimitItem> {
    let concurrent_map_len =
        concurrent_limit_opt.as_ref().map(|x| x.len()).unwrap_or(0);
    let staggered_map_len =
        staggered_limit_opt.as_ref().map(|x| x.len()).unwrap_or(0);
    let mut limits: HashMap<String, LimitItem> =
        HashMap::with_capacity(concurrent_map_len.max(staggered_map_len));

    if let Some(concurrent_limit) = concurrent_limit_opt {
        for (name, concurrent) in concurrent_limit {
            limits.insert(
                name.clone(),
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
                .entry(name.clone())
                .and_modify(|limit_item| {
                    limit_item.staggered_interval = Some(interval.0)
                })
                .or_insert_with(|| LimitItem {
                    name,
                    concurrent: None,
                    staggered_interval: Some(interval.0),
                });
        }
    }
    limits.into_values()
}

fn construct_static_tree_d(
    cfg: &config::Config,
) -> Result<HashMap<u64, PathBuf>> {
    let mut trees = HashMap::new();
    if let Some(c) = cfg.processors.static_tree_d.as_ref() {
        for (k, v) in c {
            let b = Byte::from_str(k)
                .with_context(|| format!("invalid bytes string {}", k))?;
            let size = b.get_bytes() as u64;

            seal_types_from_u64(size)
                .with_context(|| format!("invalid sector size {}", k))?;

            let tree_path = PathBuf::from(v.to_owned())
                .canonicalize()
                .with_context(|| {
                    format!("invalid tree_d path {} for sector size {}", v, k)
                })?;

            trees.insert(size, tree_path);
        }
    }

    Ok(trees)
}

macro_rules! construct_sub_processor {
    ($field:ident, $cfg:ident, $limit:ident) => {
        CtrlProc::new(if let Some(ext) = $cfg.processors.$field.as_ref() {
            let proc =
                processor::external::start_sub_processors(ext, $limit.clone())?;
            Either::Left(proc)
        } else {
            Either::Right(NoLockProcessor::new(
                Box::<BuiltinProcessor>::default(),
            ))
        })
    };
}

fn start_processors(
    cfg: &config::Config,
    limit: Arc<SealingLimit>,
) -> Result<GlobalProcessors> {
    Ok(GlobalProcessors {
        add_pieces: construct_sub_processor!(add_pieces, cfg, limit),
        tree_d: construct_sub_processor!(tree_d, cfg, limit),
        pc1: construct_sub_processor!(pc1, cfg, limit),
        pc2: construct_sub_processor!(pc2, cfg, limit),
        c2: construct_sub_processor!(c2, cfg, limit),
        snap_encode: construct_sub_processor!(snap_encode, cfg, limit),
        snap_prove: construct_sub_processor!(snap_prove, cfg, limit),
        transfer: construct_sub_processor!(transfer, cfg, limit),
        unseal: construct_sub_processor!(unseal, cfg, limit),
        window_post: construct_sub_processor!(window_post, cfg, limit),
    })
}

fn compatible_for_piece_token(cfg: &mut config::Config) {
    use vc_processors::builtin::processors::piece::fetcher::http::PieceHttpFetcher;

    if let Some(token) = &cfg.sector_manager.piece_token {
        match &mut cfg.processors.add_pieces {
            Some(add_piece_cfgs) => add_piece_cfgs.iter_mut().for_each(|cfg| {
                cfg.envs
                    .get_or_insert_with(|| HashMap::with_capacity(1))
                    .insert(
                        PieceHttpFetcher::ENV_KEY_PIECE_FETCHER_TOKEN
                            .to_string(),
                        token.clone(),
                    );
            }),
            None => env::set_var(
                PieceHttpFetcher::ENV_KEY_PIECE_FETCHER_TOKEN,
                token,
            ),
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
                vec![
                    ("pc1", None, Some(parse_duration("1s").unwrap())),
                    ("pc2", Some(20), None),
                ],
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
            let concurrent_limit_map_opt = concurrent_limit.map(|x| {
                x.into_iter()
                    .map(|(name, x)| (name.to_string(), x))
                    .collect()
            });
            let staggered_limit_map_opt = staggered_limit.map(|x| {
                x.into_iter()
                    .map(|(name, dur)| {
                        (
                            name.to_string(),
                            SerdeDuration(parse_duration(dur).unwrap()),
                        )
                    })
                    .collect()
            });
            let merged = merge_limit_config(
                concurrent_limit_map_opt.clone(),
                staggered_limit_map_opt.clone(),
            );

            let mut expect = result
                .iter()
                .map(|x| LimitItem {
                    name: x.0.to_string(),
                    concurrent: x.1,
                    staggered_interval: x.2,
                })
                .collect::<Vec<_>>();
            let mut actual = merged.collect::<Vec<_>>();

            expect.sort_by(|x, y| x.name.cmp(&y.name));
            actual.sort_by(|x, y| x.name.cmp(&y.name));

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
