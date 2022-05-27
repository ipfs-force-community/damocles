use std::io::Read;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use fil_types::UnpaddedPieceSize;
use forest_cid::Cid;
use reqwest::{
    blocking::{Client, ClientBuilder},
    header::{AUTHORIZATION, LOCATION},
    redirect::Policy,
    Url,
};

use super::PieceStore;
use crate::util::reader::inflator;

const ENDPOINT: &str = "/piecestore/";
const HEADER_AUTHORIZATION_BEARER_PREFIX: &str = "Bearer";

pub struct ProxyPieceStore {
    base: Url,
    token: Option<String>,
    client: Client,
    redirect_client: Client,
}

impl ProxyPieceStore {
    pub fn new(host: &str, token: Option<String>) -> Result<Self> {
        let h = Url::parse(host).context("parse host")?;
        let base = h.join(ENDPOINT).context("build endpoint")?;

        let client = ProxyPieceStore::build_http_client(Policy::none())?;
        let redirect_client = ProxyPieceStore::build_http_client(Policy::default())?;
        Ok(Self {
            base,
            token,
            client,
            redirect_client,
        })
    }

    fn build_http_client(policy: Policy) -> Result<Client> {
        ClientBuilder::new()
        // handle redirect ourselves
        .redirect(policy)
        .tcp_keepalive(Duration::from_secs(120))
        .connect_timeout(Duration::from_secs(5))
        .connection_verbose(true)
        .pool_max_idle_per_host(10)
        .build()
        .context("build redirect http client")
    }
}

impl PieceStore for ProxyPieceStore {
    fn get(&self, c: Cid, payload_size: u64, target_size: UnpaddedPieceSize) -> Result<Box<dyn Read>> {
        let url = self.base.join(&c.to_string()).context("invalid url")?;
        let mut resp = self.client.get(url).send().context("request to peicestore")?;

        let mut status_code = resp.status();
        if status_code.is_redirection() {
            let redirect_url = resp
                .headers()
                .get(LOCATION)
                .context("redirect location not found")
                .and_then(|val| val.to_str().context("convert redirect location to str"))
                .and_then(|s| Url::parse(s).context("parse redirect url"))?;

            let mut req = self.redirect_client.get(redirect_url);
            if let Some(token) = self.token.as_ref() {
                req = req.header(AUTHORIZATION, format!("{} {}", HEADER_AUTHORIZATION_BEARER_PREFIX, token))
            };
            resp = req.send().context("request to redirected location")?;
            status_code = resp.status();
        }

        if !status_code.is_success() {
            return Err(anyhow!("get resource {} failed invalid status code {}", c, status_code));
        }

        let r = inflator(resp, payload_size, target_size).context("build inflator reader")?;
        Ok(Box::new(r))
    }
}
