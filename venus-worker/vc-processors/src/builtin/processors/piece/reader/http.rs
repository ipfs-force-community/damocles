use std::{env, time::Duration};

use anyhow::{anyhow, Context};
use lazy_static::lazy_static;
use reqwest::{
    blocking::{Client, ClientBuilder, Response},
    header, redirect,
};

use super::PieceReader;

pub fn reader_ref() -> &'static PieceHttpClient {
    &*PIECE_HTTP_CLIENT
}

lazy_static! {
    static ref PIECE_HTTP_CLIENT: PieceHttpClient = PieceHttpClient::from_env().unwrap();
}

pub struct PieceHttpClient {
    client: Client,
    redirect_client: Client,
    token: Option<String>,
}

impl PieceReader for PieceHttpClient {
    type P = String;
    type Err = anyhow::Error;
    type Read = Response;

    fn open(&self, u: Self::P) -> Result<Self::Read, Self::Err> {
        let mut resp = self.client.get(&u).send().context("request piece url")?;

        let mut status_code = resp.status();
        if status_code.is_redirection() {
            let redirect_location = resp
                .headers()
                .get(header::LOCATION)
                .context("redirect location not found")
                .and_then(|val| val.to_str().context("convert redirect location to str"))?;

            let mut req = self.redirect_client.get(redirect_location);
            if let Some(token) = self.token.as_ref() {
                req = req.header(
                    header::AUTHORIZATION,
                    format!("{} {}", Self::HEADER_AUTHORIZATION_BEARER_PREFIX, token),
                )
            };
            resp = req.send().context("request to redirected location")?;
            status_code = resp.status();
        }

        if !status_code.is_success() {
            return Err(anyhow!("get resource {} failed invalid status code {}", u, status_code));
        }

        Ok(resp)
    }
}

impl PieceHttpClient {
    const HEADER_AUTHORIZATION_BEARER_PREFIX: &'static str = "Bearer";

    fn from_env() -> anyhow::Result<Self> {
        let token = env::var("PIECE_STORE_TOKEN").ok();
        Self::new(token)
    }

    fn new(token: Option<String>) -> anyhow::Result<Self> {
        fn build_http_client(policy: redirect::Policy) -> reqwest::Result<Client> {
            ClientBuilder::new()
                .redirect(policy) // handle redirect ourselves
                .tcp_keepalive(Duration::from_secs(120))
                .connect_timeout(Duration::from_secs(5))
                .connection_verbose(true)
                .pool_max_idle_per_host(10)
                .build()
        }

        let client = build_http_client(redirect::Policy::none()).context("build http client")?;
        let redirect_client = build_http_client(redirect::Policy::default()).context("build redirect http client")?;
        Ok(Self {
            client,
            redirect_client,
            token,
        })
    }
}
