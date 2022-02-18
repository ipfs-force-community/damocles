use std::net::{IpAddr, SocketAddr, TcpStream};

use anyhow::{anyhow, Result};
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
