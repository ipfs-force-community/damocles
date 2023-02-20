use std::fs::{create_dir, create_dir_all, remove_dir_all, remove_file, File, OpenOptions};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};

use hex::encode;
use pretty_assertions::assert_eq;
use rand::{rngs::OsRng, thread_rng, RngCore};

use super::{
    super::{
        super::tasks::{TransferItem, TransferOption},
        TransferRoute,
    },
    do_transfer, ensure_dest_parent,
};

struct TempDir(PathBuf);

impl TempDir {
    fn new() -> Self {
        let mut buf = [0u8; 8];
        OsRng.fill_bytes(&mut buf);

        let dir = std::env::temp_dir().join(format!("tests-{}", encode(buf)));

        create_dir_all(&dir).expect("create tmp dir");

        TempDir(dir)
    }

    fn touch<P: AsRef<Path>>(&self, rel: P, data: Option<&[u8]>) -> PathBuf {
        let p = self.0.join(rel);
        ensure_dest_parent(&p).expect("ensure parent");
        let mut f = OpenOptions::new().create_new(true).write(true).open(&p).expect("touch file");
        if let Some(b) = data {
            f.write_all(b).expect("write data");
        }
        p
    }

    fn create_dir<P: AsRef<Path>>(&self, rel: P) -> PathBuf {
        let p = self.0.join(rel);
        ensure_dest_parent(&p).expect("ensure parent");
        create_dir(&p).expect("create dir");
        p
    }
}

impl Drop for TempDir {
    fn drop(&mut self) {
        remove_dir_all(&self.0).expect("remove tmp dir");
    }
}

fn compare_files(left: &Path, right: &Path) {
    let mut left_data = Vec::new();
    let mut right_data = Vec::new();
    File::open(left)
        .unwrap_or_else(|_| panic!("open {:?}", left))
        .read_to_end(&mut left_data)
        .unwrap_or_else(|_| panic!("read {:?}", left));

    File::open(right)
        .unwrap_or_else(|_| panic!("open {:?}", right))
        .read_to_end(&mut right_data)
        .unwrap_or_else(|_| panic!("read {:?}", right));

    assert_eq!(left_data, right_data, "compare file data {:?} vs {:?}", left, right);
}

#[test]
fn transfer_failure_test() {
    let tmp = TempDir::new();

    let rel_src = "src/dir";
    let rel_dest = "dest/dir";

    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: rel_src.into(),
        },
        dest: TransferItem {
            store_name: None,
            uri: rel_dest.into(),
        },
        opt: None,
    });
    assert!(
        res.unwrap_err().to_string().contains("src path is relative"),
        "src path is relative"
    );

    let src_path = tmp.0.join(rel_src);
    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path.clone(),
        },
        dest: TransferItem {
            store_name: None,
            uri: rel_dest.into(),
        },
        opt: None,
    });
    assert!(res.unwrap_err().to_string().contains("src path not exists"), "src path not exists");

    tmp.create_dir(rel_src);
    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path.clone(),
        },
        dest: TransferItem {
            store_name: None,
            uri: "b/dir".into(),
        },
        opt: None,
    });
    assert!(
        res.unwrap_err().to_string().contains("dest path is relative"),
        "dest path is relative"
    );

    let dest_path = tmp.touch(rel_dest, None);
    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path.clone(),
        },
        dest: TransferItem {
            store_name: None,
            uri: dest_path.clone(),
        },
        opt: None,
    });
    assert!(
        res.unwrap_err().to_string().contains("dest entry type is different with src"),
        "dest entry type is different with src"
    );

    remove_file(&dest_path).expect("remove dest file");
    create_dir_all(&dest_path).expect("create dest dir");

    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path,
        },
        dest: TransferItem {
            store_name: None,
            uri: dest_path,
        },
        opt: None,
    });

    assert!(res.is_ok(), "ok");
}

#[test]
fn transfer_test() {
    let tmp = TempDir::new();
    let mut rng = thread_rng();

    let rel_src_dir = "src";
    let rel_dest_dir = "dest";

    let src_path = tmp.0.join(rel_src_dir);
    let dest_path = tmp.0.join(rel_dest_dir);

    let mut data = [0u8; 32];
    let file_pairs: Vec<_> = (1..=5)
        .map(|idx: usize| {
            let rel_f = PathBuf::from(rel_src_dir).join(idx.to_string());
            rng.fill_bytes(&mut data);
            let src_file_path = tmp.touch(rel_f, Some(&data[..]));
            (src_file_path, tmp.0.join(rel_dest_dir).join(idx.to_string()))
        })
        .collect();

    // link dir
    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path.clone(),
        },
        dest: TransferItem {
            store_name: None,
            uri: dest_path.clone(),
        },
        opt: Some(TransferOption {
            is_dir: true,
            allow_link: true,
        }),
    });

    assert!(res.is_ok(), "transfer dir {:?} to {:?}, allow_link", &src_path, &dest_path);

    assert!(dest_path.is_symlink(), "dest dir should be symlink");
    for (_, dfp) in file_pairs.iter() {
        assert!(!dfp.is_symlink(), "file {:?} in dest dir should not be symlink", dfp);
    }

    // link files
    remove_dir_all(&dest_path).expect("clean up dest dir");
    for (sfp, dfp) in file_pairs.iter() {
        let res = do_transfer(&TransferRoute {
            src: TransferItem {
                store_name: None,
                uri: sfp.clone(),
            },
            dest: TransferItem {
                store_name: None,
                uri: dfp.clone(),
            },
            opt: Some(TransferOption {
                is_dir: false,
                allow_link: true,
            }),
        });
        assert!(res.is_ok(), "transfer file {:?} to {:?}, allow_link", sfp, dfp);
    }

    assert!(!dest_path.is_symlink(), "dest dir should not be symlink");
    for (_, dfp) in file_pairs.iter() {
        assert!(dfp.is_symlink(), "file {:?} in dest dir should be symlink", dfp);
    }

    // copy dir
    remove_dir_all(&dest_path).expect("clean up dest dir");
    let res = do_transfer(&TransferRoute {
        src: TransferItem {
            store_name: None,
            uri: src_path.clone(),
        },
        dest: TransferItem {
            store_name: None,
            uri: dest_path.clone(),
        },
        opt: None,
    });

    assert!(res.is_ok(), "transfer dir {:?} to {:?}, disallow_link", &src_path, &dest_path);
    assert!(!dest_path.is_symlink(), "dest dir should not be symlink");
    for (sfp, dfp) in file_pairs.iter() {
        assert!(!dfp.is_symlink(), "file {:?} in dest dir should not be symlink", dfp);
        compare_files(sfp, dfp);
    }

    // copy files
    remove_dir_all(&dest_path).expect("clean up dest dir");
    for (sfp, dfp) in file_pairs.iter().take(3) {
        let res = do_transfer(&TransferRoute {
            src: TransferItem {
                store_name: None,
                uri: sfp.clone(),
            },
            dest: TransferItem {
                store_name: None,
                uri: dfp.clone(),
            },
            opt: None,
        });
        assert!(res.is_ok(), "transfer file {:?} to {:?}, disallow_link", sfp, dfp);
    }

    assert!(!dest_path.is_symlink(), "dest dir should not be symlink");

    for (sfp, dfp) in file_pairs.iter().take(3) {
        assert!(!dfp.is_symlink(), "file {:?} in dest dir should not be symlink", dfp);
        compare_files(sfp, dfp);
    }

    let mut non_exist_count = 0;
    for (sfp, dfp) in file_pairs.iter().skip(3) {
        non_exist_count += 1;
        assert!(sfp.exists(), "src file {:?} exist", sfp);
        assert!(!dfp.exists(), "dest file {:?} should not exist", dfp);
    }

    assert_eq!(non_exist_count, 2);
}
