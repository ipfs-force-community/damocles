use std::convert::TryFrom;
use std::net::{IpAddr, SocketAddr, TcpStream};

use async_tungstenite::tungstenite::http::Uri;

use anyhow::{anyhow, Result};

pub fn local_interface_ip(dest: SocketAddr) -> Result<IpAddr> {
    let stream = TcpStream::connect(dest)?;
    let addr = stream.local_addr()?;
    Ok(addr.ip())
}

pub fn socket_addr_from_url(u: &str) -> Result<SocketAddr> {
    let uri = Uri::try_from(u)?;
    let host = uri
        .host()
        .ok_or(anyhow!("host is required in the target url"))?;

    let port = match uri.port_u16() {
        Some(p) => p,
        None => match uri.scheme_str() {
            Some("https") | Some("wss") => 443,
            Some("http") | Some("ws") => 80,
            Some(other) => return Err(anyhow!("unknown scheme {} str in url", other)),
            None => return Err(anyhow!("no scheme str in the target url")),
        },
    };

    let addr = format!("{}:{}", host, port).parse()?;

    Ok(addr)
}
