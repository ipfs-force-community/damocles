use std::net::{IpAddr, SocketAddr, TcpStream};

use anyhow::{anyhow, Context, Result};
use multiaddr::{Multiaddr, Protocol};
use reqwest::Url;

pub fn local_interface_ip(dest: &[SocketAddr]) -> Result<IpAddr> {
    let stream = TcpStream::connect(dest)?;
    let addr = stream.local_addr()?;
    Ok(addr.ip())
}

pub fn rpc_addr(raw: &str, ver: u32) -> Result<String> {
    let multiaddr_err = match raw
        .parse()
        .context("try to parse multiaddr")
        .and_then(|ma| rpc_addr_from_multiaddr(ma, ver))
    {
        Ok(addr) => return Ok(addr),
        Err(e) => e,
    };

    let url_err =
        match Url::parse(raw).context("try to parse url").and_then(|url| {
            Ok(format_rpc_addr(
                url.scheme(),
                url.host_str().context("url host is required")?,
                url.port(),
                ver,
            ))
        }) {
            Ok(addr) => return Ok(addr),
            Err(e) => e,
        };

    Err(anyhow!(
        "parse rpc addr: `{}`\n1. {:?}\n\n2. {:?}",
        raw,
        multiaddr_err,
        url_err
    ))
}

#[inline]
fn format_rpc_addr(
    scheme: &str,
    host: &str,
    port: Option<u16>,
    ver: u32,
) -> String {
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

            Protocol::Dns(h) | Protocol::Dns4(h) | Protocol::Dns6(h) => {
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
        &host.context("try to parse multiaddr: host is required")?,
        port,
        ver,
    ))
}

#[cfg(test)]
mod tests {
    use pretty_assertions::assert_eq;

    #[test]
    fn test_rpc_addr() {
        let cases = vec![
            ("/ip4/127.0.0.1/tcp/1789", 0, "http://127.0.0.1:1789/rpc/v0"),
            (
                "/ip6/2001:db8:3333:4444:5555:6666:7777:8888/tcp/1789",
                0,
                "http://2001:db8:3333:4444:5555:6666:7777:8888:1789/rpc/v0",
            ),
            ("/dns/127.0.0.1/tcp/1789", 0, "http://127.0.0.1:1789/rpc/v0"),
            (
                "/dns4/hello.com/tcp/1789",
                0,
                "http://hello.com:1789/rpc/v0",
            ),
            (
                "/dns6/hello.com/tcp/1789",
                0,
                "http://hello.com:1789/rpc/v0",
            ),
            ("http://hello.com:1789", 0, "http://hello.com:1789/rpc/v0"),
            (
                "/ip4/127.0.0.1/tcp/17890",
                1,
                "http://127.0.0.1:17890/rpc/v1",
            ),
            (
                "/ip6/2001:db8:3333:4444:5555:6666:7777:8888/tcp/17890",
                1,
                "http://2001:db8:3333:4444:5555:6666:7777:8888:17890/rpc/v1",
            ),
            (
                "/dns/127.0.0.1/tcp/17890",
                1,
                "http://127.0.0.1:17890/rpc/v1",
            ),
            (
                "/dns4/hello.com/tcp/17890",
                1,
                "http://hello.com:17890/rpc/v1",
            ),
            (
                "/dns6/hello.com/tcp/17890",
                1,
                "http://hello.com:17890/rpc/v1",
            ),
            ("http://hello.com:17890", 1, "http://hello.com:17890/rpc/v1"),
        ];

        for (raw, ver, expected) in cases {
            let actual = super::rpc_addr(raw, ver).expect("never fail");
            assert_eq!(
                actual,
                expected.to_string(),
                "testing for `raw: {}, ver: {}`",
                raw,
                ver
            );
        }
    }
}
