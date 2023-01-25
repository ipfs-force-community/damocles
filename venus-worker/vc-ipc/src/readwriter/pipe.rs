use std::{
    ffi::{OsStr, OsString},
    io::{self, Result},
    iter::{self, FlatMap, Map},
    option,
    path::Path,
    pin::Pin,
    process::{self, Stdio},
    slice,
    task::{Context, Poll},
};

use futures::future::{ready, Ready};
use mdsn::Dsn;
use pin_project::pin_project;
use tokio::{
    io::{stdin, stdout, AsyncRead, AsyncWrite, ReadBuf, Stdin, Stdout},
    process::{ChildStdin, ChildStdout, Command},
};

use super::reconnect::Reconnectable;

#[pin_project]
pub struct Pipe<R, W> {
    id: u32,
    #[pin]
    reader: R,
    #[pin]
    writer: W,
}

/// Executes the command `bin` as a child process and establish pipe connection,
///
/// # Example
/// ```
/// use std::{error::Error, ffi::OsString, iter::empty, path::PathBuf};
/// use tokio::io::{AsyncReadExt, AsyncWriteExt};
///
/// use vc_ipc::readwriter::{pipe::connect};
///
/// #[tokio::main]
/// async fn main() -> Result<(), Box<dyn Error>> {
///     let mut cat = connect(PathBuf::from("/bin/cat"), empty::<OsString>())?;
///     // Write some data.
///     cat.write_all(b"hello world").await?;
///
///     let mut buf = vec![0; 11];
///     cat.read_exact(&mut buf).await?;
///     assert_eq!(b"hello world", buf.as_slice());
///     Ok(())
/// }
/// ```
pub fn connect(bin: impl AsRef<Path>, args: impl IntoIterator<Item = impl AsRef<OsStr>>) -> io::Result<Pipe<ChildStdout, ChildStdin>> {
    let cmd = Command::new(bin.as_ref())
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .args(args)
        .spawn()?;

    match (cmd.id(), cmd.stdout, cmd.stdin) {
        (Some(id), Some(reader), Some(writer)) => Ok(Pipe { id, reader, writer }),
        _ => Err(io::Error::new(io::ErrorKind::BrokenPipe, "child process exited unexpectedly")),
    }
}

/// Listen in standard I/O
pub fn listen() -> Pipe<Stdin, Stdout> {
    Pipe {
        id: process::id(),
        reader: stdin(),
        writer: stdout(),
    }
}

pub trait IntoPipeConnectArgs {
    type Path<'a>: AsRef<Path> + 'a
    where
        Self: 'a;

    type Arg<'a>: AsRef<OsStr> + 'a
    where
        Self: 'a;

    type ArgsIt<'a>: IntoIterator<Item = Self::Arg<'a>>
    where
        Self: 'a;

    fn bin_path(&self) -> Option<Self::Path<'_>>;
    fn args(&self) -> Self::ArgsIt<'_>;
}

impl<'a> IntoPipeConnectArgs for &'a Path {
    type Path<'b> = &'b Path where 'a:'b;

    type Arg<'b> = &'b OsString where 'a:'b;

    type ArgsIt<'b> = iter::Empty<&'b OsString> where 'a:'b;

    fn bin_path(&self) -> Option<Self::Path<'_>> {
        Some(self)
    }

    fn args(&self) -> Self::ArgsIt<'_> {
        iter::empty()
    }
}

impl<'a> IntoPipeConnectArgs for (&'a Path, &'a [OsString]) {
    type Path<'b> = &'b Path where 'a:'b;

    type Arg<'b> = &'b OsString where 'a:'b;

    type ArgsIt<'b> = slice::Iter<'b, OsString> where 'a:'b;

    fn bin_path(&self) -> Option<Self::Path<'_>> {
        Some(self.0)
    }

    fn args(&self) -> Self::ArgsIt<'_> {
        self.1.iter()
    }
}

impl IntoPipeConnectArgs for Dsn {
    type Path<'a> = &'a Path;

    type Arg<'a> = OsString;

    type ArgsIt<'a> = FlatMap<
        option::IntoIter<&'a String>,
        Map<shlex::Shlex<'a>, fn(String) -> OsString>,
        fn(&'a String) -> Map<shlex::Shlex<'a>, fn(String) -> OsString>,
    >;

    fn bin_path(&self) -> Option<Self::Path<'_>> {
        self.path.as_ref().map(Path::new)
    }

    fn args(&self) -> Self::ArgsIt<'_> {
        self.params
            .get("args")
            .into_iter()
            .flat_map(|args| shlex::Shlex::new(args).map(OsString::from))
    }
}

impl<C> Reconnectable<C> for Pipe<ChildStdout, ChildStdin>
where
    C: IntoPipeConnectArgs,
{
    type ConnectingFut = Ready<io::Result<Self>>;

    fn connect(ctor_arg: C) -> Self::ConnectingFut {
        match ctor_arg.bin_path() {
            Some(bin_path) => ready(connect(bin_path, ctor_arg.args())),
            None => ready(Err(io::Error::new(io::ErrorKind::NotFound, "bin path cannot be empty"))),
        }
    }
}

impl<R, W> AsyncRead for Pipe<R, W>
where
    R: AsyncRead,
{
    fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut ReadBuf<'_>) -> Poll<Result<()>> {
        self.project().reader.poll_read(cx, buf)
    }
}

impl<R, W> AsyncWrite for Pipe<R, W>
where
    W: AsyncWrite,
{
    fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<Result<usize>> {
        self.project().writer.poll_write(cx, buf)
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<()>> {
        self.project().writer.poll_flush(cx)
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<()>> {
        self.project().writer.poll_shutdown(cx)
    }
}

#[cfg(test)]
mod tests {
    use std::{path::Path, str::FromStr};

    use mdsn::Dsn;
    use pretty_assertions::assert_eq;

    use super::IntoPipeConnectArgs;

    #[test]
    fn dsn_to_pipe_connect_args() {
        let cases = vec![
            ("pipe+length+json:./xxx/test.bin", Some("./xxx/test.bin"), vec![]),
            (
                "x:/test.bin?args=--aa=bb --flag subcommand --cc dd",
                Some("/test.bin"),
                vec!["--aa=bb", "--flag", "subcommand", "--cc", "dd"],
            ),
            (
                "x:/test.bin?args=aa=%26%26 --flag ' xx%26 %26ee'",
                Some("/test.bin"),
                vec!["aa=&&", "--flag", " xx& &ee"],
            ),
        ];

        for (dsn, expected_bin_path, expected_args) in cases {
            let u = Dsn::from_str(dsn).unwrap();
            assert_eq!(u.bin_path(), expected_bin_path.map(Path::new), "test for {}", dsn);
            let actual_args: Vec<_> = u.args().collect();
            assert_eq!(actual_args, expected_args, "test for {}", dsn);
        }
    }
}
