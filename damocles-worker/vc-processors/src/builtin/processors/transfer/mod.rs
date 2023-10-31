use std::fs::{create_dir_all, remove_dir_all, remove_file, OpenOptions};
use std::io::copy;
use std::os::unix::fs::symlink;
use std::path::Path;

use anyhow::{anyhow, Context, Result};
use tracing::info;

use super::TransferRoute;

#[cfg(test)]
mod tests;

pub fn do_transfer(route: &TransferRoute) -> Result<()> {
    do_transfer_inner(route, false)
}

pub fn do_transfer_inner(
    route: &TransferRoute,
    disable_link: bool,
) -> Result<()> {
    if route.src.uri.is_relative() {
        return Err(anyhow!("src path is relative"));
    }

    if !route.src.uri.exists() {
        return Err(anyhow!(
            "src path not exists: {}",
            route.src.uri.display()
        ));
    }

    let src_is_dir = route.src.uri.is_dir();

    if route.dest.uri.is_relative() {
        return Err(anyhow!("dest path is relative"));
    }

    if route.dest.uri.exists() {
        let dest_is_dir = route.dest.uri.is_dir();
        if src_is_dir != dest_is_dir {
            return Err(anyhow!(
                "dest entry type is different with src, is_dir={}",
                src_is_dir
            ));
        }

        if dest_is_dir {
            remove_dir_all(&route.dest.uri).with_context(|| {
                format!("remove exist dest dir: {}", route.dest.uri.display())
            })?;
        } else {
            remove_file(&route.dest.uri).with_context(|| {
                format!("remove exist dest file: {}", route.dest.uri.display())
            })?;
        }
    }

    if !disable_link {
        if let Some(true) = route.opt.as_ref().map(|opt| opt.allow_link) {
            link_entry(&route.src.uri, &route.dest.uri)
                .context("link entry")?;
            info!(src=?&route.src.uri, dest=?&route.dest.uri, "entry linked");
            return Ok(());
        }
    }

    if src_is_dir {
        copy_dir(&route.src.uri, &route.dest.uri).with_context(|| {
            format!(
                "transfer dir {:?} to {:?}",
                &route.src.uri, &route.dest.uri
            )
        })?;
        info!(src=?&route.src.uri, dest=?&route.dest.uri, "dir copied");
    } else {
        let size =
            copy_file(&route.src.uri, &route.dest.uri).with_context(|| {
                format!(
                    "transfer file {:?} to {:?}",
                    &route.src.uri, &route.dest.uri
                )
            })?;
        info!(src=?&route.src.uri, dest=?&route.dest.uri, size, "file copied");
    }

    Ok(())
}

fn ensure_dest_parent(dest: &Path) -> Result<()> {
    if let Some(parent) = dest.parent() {
        create_dir_all(parent)
            .with_context(|| format!("ensure dest parent for {:?}", dest))?;
    }

    Ok(())
}

fn link_entry(src: &Path, dest: &Path) -> Result<()> {
    ensure_dest_parent(dest)?;
    symlink(src, dest)?;
    Ok(())
}

fn copy_file(src: &Path, dest: &Path) -> Result<u64> {
    ensure_dest_parent(dest)?;
    let mut r = OpenOptions::new()
        .read(true)
        .open(src)
        .context("open src file")?;
    let mut f = OpenOptions::new()
        .read(true)
        .write(true)
        .create(true)
        .truncate(true)
        .open(dest)
        .context("create dest file")?;
    copy(&mut r, &mut f).context("copy from src to dest")
}

fn copy_dir(src: &Path, dest: &Path) -> Result<()> {
    ensure_dest_parent(dest)?;

    for entry_res in src.read_dir().context("read src dir")? {
        let entry = entry_res.context("entry error in src dir")?;
        let full_path = entry.path();
        let rel_path = full_path
            .strip_prefix(src)
            .with_context(|| format!("get rel path for {:?}", full_path))?;

        let target = dest.join(rel_path);
        if full_path.is_dir() {
            copy_dir(&full_path, &target).with_context(|| {
                format!("copy dir inside {:?} to {:?}", full_path, dest)
            })?;
        } else {
            copy_file(&full_path, &target).with_context(|| {
                format!("copy file inside {:?} to {:?}", full_path, dest)
            })?;
        }
    }

    Ok(())
}
