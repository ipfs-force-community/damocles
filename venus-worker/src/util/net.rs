use std::net::{IpAddr, SocketAddr, TcpStream};

use anyhow::{anyhow, Context, Result};
use multiaddr::{Multiaddr, Protocol};
use reqwest::Url;

pub fn local_interface_ip(dest: SocketAddr) -> Result<IpAddr> {
    let stream = TcpStream::connect(dest)?;
    let addr = stream.local_addr()?;
    Ok(addr.ip())
}

pub fn socket_addr_from_url(u: &str) -> Result<SocketAddr> {
    let url = Url::parse(u)?;
    let host = url
        .host_str()
        .ok_or(anyhow!("host is required in the target url"))?;

    let port = match url.port_or_known_default() {
        Some(p) => p,
        None => return Err(anyhow!("no known port for the url")),
    };

    let addr = format!("{}:{}", host, port).parse()?;

    Ok(addr)
}

pub fn rpc_addr(raw: &str, ver: u32) -> Result<String> {
    if let Ok(ma) = raw.parse() {
        return rpc_addr_from_multiaddr(ma, ver);
    }

    let url = Url::parse(raw).context("url is not valid")?;
    Ok(format_rpc_addr(
        url.scheme(),
        url.host_str().context("url host is required")?,
        url.port(),
        ver,
    ))
}

#[inline]
fn format_rpc_addr(scheme: &str, host: &str, port: Option<u16>, ver: u32) -> String {
    match port {
        Some(p) => {
            format!("{}://{}:{}/rpc/v{}", scheme, host, p, ver)
        }

        None => {
            format!("{}://{}/rpc/v{}", scheme, host, ver)
        }
    }
}

#[inline]
fn rpc_addr_from_multiaddr(ma: Multiaddr, ver: u32) -> Result<String> {
    let mut scheme = None;
    let mut host = None;
    let mut port = None;

    for protocol in ma.into_iter() {
        match protocol {
            Protocol::Ip4(h) => {
                host.replace(h.to_string());
            }

            Protocol::Ip6(h) => {
                host.replace(h.to_string());
            }

            Protocol::Tcp(p) => {
                port.replace(p);
            }

            Protocol::Http => {
                scheme.replace("http");
            }

            Protocol::Https => {
                scheme.replace("https");
            }

            _ => {}
        }
    }

    Ok(format_rpc_addr(
        scheme.unwrap_or("http"),
        &host.context("ip4 or ip6 protocol is required")?,
        port,
        ver,
    ))
}
