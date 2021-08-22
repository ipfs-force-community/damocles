use std::collections::HashMap;
use std::sync::Arc;
use std::thread;

use anyhow::{anyhow, Result};
use crossbeam_channel::{bounded, Receiver, Select, Sender};

/// return done tx & rx
pub fn dones() -> (Sender<()>, Receiver<()>) {
    bounded(0)
}

use crate::{
    config::Config,
    infra::objstore::ObjectStore,
    logging::{error, error_span, info, warn},
    rpc::sealer::SealerClient,
    sealing::{
        resource::Pool,
        seal::{BoxedC2Processor, BoxedPC2Processor},
    },
};

pub type Done = Receiver<()>;

#[derive(Clone)]
pub struct Ctx {
    pub done: Done,
    pub cfg: Arc<Config>,
    pub instance: String,
    pub global: GlobalModules,
}

#[derive(Clone)]
pub struct GlobalModules {
    pub rpc: Arc<SealerClient>,
    pub remote_store: Arc<Box<dyn ObjectStore>>,
    pub pc2: Arc<BoxedPC2Processor>,
    pub c2: Arc<BoxedC2Processor>,
    pub limit: Arc<Pool>,
}

pub trait Module: Send {
    fn id(&self) -> String;
    fn run(&mut self, ctx: Ctx) -> Result<()>;
}

pub struct WatchDog {
    pub ctx: Ctx,
    done_ctrl: Option<Sender<()>>,
    modules: Vec<(String, thread::JoinHandle<()>, Receiver<Result<()>>)>,
}

impl WatchDog {
    pub fn build(cfg: Config, instance: String, global: GlobalModules) -> Self {
        Self::build_with_done(cfg, instance, global, dones())
    }

    pub fn build_with_done(
        cfg: Config,
        instance: String,
        global: GlobalModules,
        done: (Sender<()>, Receiver<()>),
    ) -> Self {
        Self {
            ctx: Ctx {
                done: done.1,
                instance,
                cfg: Arc::new(cfg),
                global,
            },
            done_ctrl: Some(done.0),
            modules: Vec::new(),
        }
    }

    pub fn start_module(&mut self, m: impl 'static + Module) {
        let ctx = self.ctx.clone();
        let id = m.id();
        let (res_tx, res_rx) = bounded(1);
        let hdl = thread::spawn(move || {
            let mut m = m;
            let id = m.id();
            let span = error_span!("module", name = id.as_str());
            let _guard = span.enter();
            info!("start");
            let res = m.run(ctx);
            info!("stop");
            let _ = res_tx.send(res);
        });

        self.modules.push((id, hdl, res_rx));
    }

    pub fn wait(&mut self) -> Result<()> {
        if self.modules.is_empty() {
            return Ok(());
        }

        let done_ctrl = self
            .done_ctrl
            .take()
            .ok_or(anyhow!("no done controller provided"));

        let mut indexes = HashMap::new();
        let mut selector = Select::new();
        for (i, m) in self.modules.iter().enumerate() {
            let idx = selector.recv(&m.2);
            indexes.insert(idx, i);
        }

        let op = selector.select();
        let opidx = op.index();
        let midx = match indexes.get(&opidx).cloned() {
            None => return Err(anyhow!("no module found for select op index {}", opidx)),
            Some(i) => i,
        };

        let mname = (self.modules[midx].0).as_str();
        let res = match op.recv(&self.modules[midx].2) {
            Ok(r) => r,
            Err(e) => {
                return Err(anyhow!(
                    "unable to recv run result from module {} from chan: {}",
                    mname,
                    e
                ))
            }
        };

        match res {
            Ok(_) => {
                warn!("module {} stopped", mname);
            }
            Err(e) => {
                error!("module {} stopped unexpectedly: {:?}", mname, e);
            }
        }
        drop(done_ctrl);

        // TODO: wait for all submodules to stop gracefully

        Ok(())
    }
}
