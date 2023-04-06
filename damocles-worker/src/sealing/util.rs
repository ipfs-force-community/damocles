//! helper utilities for sealing

use std::{collections::HashMap, path::PathBuf};

use anyhow::{Context, Result};

/// Scan memory files by the given `memory_file_dir_pattern`
///
/// let p = MemoryFileDirPattern::new("filecoin-proof/procsssor_1/numa_$NUMA_NODE_INDEX", "/mnt/huge");
/// scan_memory_files(&p);
///
/// $NUMA_NODE_INDEX corresponds to the index of the numa node,
/// and you should make sure that the memory files stored in the folder
/// were created in the numa node corresponding to $NUMA_NODE_INDEX
///
/// In the above example, the following memory files will be matched.
///
/// NUMA node 0:
/// /mnt/huge/filecoin-proof/procsssor_1/numa_0/mem_32GiB_1
/// /mnt/huge/filecoin-proof/procsssor_1/numa_0/mem_64GiB_1
/// /mnt/huge/filecoin-proof/procsssor_1/numa_0/any_file_name
/// NUMA node 1:
/// /mnt/huge/filecoin-proof/procsssor_1/numa_1/mem_32GiB_1
/// /mnt/huge/filecoin-proof/procsssor_1/numa_1/mem_64GiB_1
/// /mnt/huge/filecoin-proof/procsssor_1/numa_1/any_file_name
/// NUMA node N:
/// ...
pub fn scan_memory_files(memory_file_dir_pattern: &MemoryFileDirPattern) -> Result<Vec<Vec<PathBuf>>> {
    use glob::glob;
    use regex::Regex;

    let re_numa_node_idx =
        Regex::new(memory_file_dir_pattern.to_regex_pattern().as_str()).context("invalid `multicore_sdr_shm_numa_dir_pattern`")?;

    // numa_memory_files_map: { NUMA_NODE_INDEX -> Vec<PathBuf of this numa node shm file> }
    let mut numa_memory_files_map = HashMap::new();
    glob(memory_file_dir_pattern.to_glob_pattern().as_str())
        .context("invalid `multicore_sdr_shm_numa_dir_pattern`")?
        .filter_map(|path_res| path_res.ok())
        .filter_map(|path| {
            let numa_node_idx: usize = re_numa_node_idx.captures(path.to_str()?)?.get(1)?.as_str().parse().ok()?;
            Some((numa_node_idx, path))
        })
        .for_each(|(numa_node_idx, path)| {
            numa_memory_files_map.entry(numa_node_idx).or_insert_with(Vec::new).push(path);
        });

    // Converts the numa_memory_files_map { NUMA_NODE_INDEX -> Vec<PathBuf of this numa node shm file> }
    // to numa_memory_files Vec [ NUMA_NODE_INDEX -> Vec<PathBuf of this numa node shm file> ]
    let numa_memory_files = match numa_memory_files_map.keys().max() {
        Some(&max_node_idx) => {
            let mut numa_vec = Vec::with_capacity(max_node_idx + 1);
            for i in 0..=max_node_idx {
                numa_vec.push(numa_memory_files_map.remove(&i).unwrap_or_default())
            }
            numa_vec
        }
        None => Vec::new(),
    };
    Ok(numa_memory_files)
}

/// The memory files directory pattern
pub struct MemoryFileDirPattern(String);

impl MemoryFileDirPattern {
    /// NUMA node index variable name in `MemoryFileDirPattern`
    pub const NUMA_NODE_IDX_VAR_NAME: &'static str = "$NUMA_NODE_INDEX";

    /// Default pattern str.
    ///
    /// /path/to/**node_0**
    /// /path/to/**node_1**
    /// ...
    pub const DEFAULT_PATTERN_STR: &'static str = "numa_$NUMA_NODE_INDEX";

    /// Creates a new MemoryFileDirPattern with given pattern and prefix
    pub fn new(pattern: &str, prefix: &str) -> Self {
        if prefix.is_empty() {
            Self(glob::Pattern::escape(pattern.trim_end_matches('/')))
        } else {
            Self(format!(
                "{}/{}",
                prefix.trim_end_matches('/'),
                glob::Pattern::escape(pattern.trim_matches('/')),
            ))
        }
    }

    /// Creates a new MemoryFileDirPattern with given prefix and the `Self::DEFAULT_PATTERN_STR`
    pub fn new_default(prefix: &str) -> Self {
        Self(format!("{}/{}", prefix.trim_end_matches('/'), Self::DEFAULT_PATTERN_STR))
    }

    /// Creates a new MemoryFileDirPattern with given pattern
    pub fn without_prefix(pattern: &str) -> Self {
        Self::new(pattern, "")
    }

    /// Converts to glob pattern
    ///
    /// # Examples
    ///
    /// ```
    /// use damocles_worker::seal_util::MemoryFileDirPattern;
    ///
    /// let p = MemoryFileDirPattern::new_default("/dev/shm/abc");
    /// assert_eq!(p.to_glob_pattern(), String::from("/dev/shm/abc/numa_*/*"));
    ///
    /// let p = MemoryFileDirPattern::new("abc/nu_$NUMA_NODE_INDEX_ma", "/mnt/huge_2m");
    /// assert_eq!(p.to_glob_pattern(), String::from("/mnt/huge_2m/abc/nu_*_ma/*"));
    ///
    /// let p = MemoryFileDirPattern::without_prefix("/mnt/huge_1g/abc/nu_$NUMA_NODE_INDEX_ma");
    /// assert_eq!(p.to_glob_pattern(), String::from("/mnt/huge_1g/abc/nu_*_ma/*"));
    ///
    /// ```
    pub fn to_glob_pattern(&self) -> String {
        format!("{}/*", self.0.replacen(Self::NUMA_NODE_IDX_VAR_NAME, "*", 1))
    }

    /// Converts to regex pattern
    ///
    /// # Examples
    ///
    /// ```
    /// use damocles_worker::seal_util::MemoryFileDirPattern;
    ///
    /// let p = MemoryFileDirPattern::new_default("/dev/shm/abc");
    /// assert_eq!(p.to_regex_pattern(), String::from(r"/dev/shm/abc/numa_(\d+)/.+"));
    ///
    /// let p = MemoryFileDirPattern::new("abc/nu_$NUMA_NODE_INDEX_ma", "/mnt/huge_2m");
    /// assert_eq!(p.to_regex_pattern(), String::from(r"/mnt/huge_2m/abc/nu_(\d+)_ma/.+"));
    ///
    /// let p = MemoryFileDirPattern::without_prefix("/mnt/huge_1g/abc/nu_$NUMA_NODE_INDEX_ma");
    /// assert_eq!(p.to_regex_pattern(), String::from(r"/mnt/huge_1g/abc/nu_(\d+)_ma/.+"));
    ///
    /// ```
    pub fn to_regex_pattern(&self) -> String {
        format!("{}/.+", self.0.replacen(Self::NUMA_NODE_IDX_VAR_NAME, "(\\d+)", 1))
    }

    /// Converts to PathBuf by given `numa_node_idx`
    ///
    /// # Example
    ///
    /// ```
    /// use std::path::PathBuf;
    ///
    /// use damocles_worker::seal_util::MemoryFileDirPattern;
    ///
    /// let p = MemoryFileDirPattern::new_default("/dev/shm/abc");
    /// assert_eq!(p.to_path(0), PathBuf::from("/dev/shm/abc/numa_0"));
    /// assert_eq!(p.to_path(2), PathBuf::from("/dev/shm/abc/numa_2"));
    ///
    /// let p = MemoryFileDirPattern::new("abc/nu_$NUMA_NODE_INDEX_ma", "/mnt/huge_2m");
    /// assert_eq!(p.to_path(0), PathBuf::from("/mnt/huge_2m/abc/nu_0_ma"));
    /// assert_eq!(p.to_path(2), PathBuf::from("/mnt/huge_2m/abc/nu_2_ma"));
    ///
    /// let p = MemoryFileDirPattern::without_prefix("/mnt/huge_1g/abc/nu_$NUMA_NODE_INDEX_ma");
    /// assert_eq!(p.to_path(0), PathBuf::from("/mnt/huge_1g/abc/nu_0_ma"));
    /// assert_eq!(p.to_path(2), PathBuf::from("/mnt/huge_1g/abc/nu_2_ma"));
    ///
    /// ```
    pub fn to_path(&self, numa_node_idx: u32) -> PathBuf {
        PathBuf::from(self.0.replacen(Self::NUMA_NODE_IDX_VAR_NAME, numa_node_idx.to_string().as_str(), 1))
    }
}

#[cfg(test)]
mod tests {
    use std::{
        collections::HashMap,
        fs,
        iter::repeat,
        path::{Path, PathBuf},
    };

    use pretty_assertions::assert_eq;
    use rand::{distributions::Alphanumeric, prelude::ThreadRng, Rng};

    use super::{scan_memory_files, MemoryFileDirPattern};

    #[test]
    fn test_scan_memory_files() {
        const NUMA_NODE_IDX_VAR_NAME: &str = MemoryFileDirPattern::NUMA_NODE_IDX_VAR_NAME;

        struct TestCase {
            memory_file_dir_pattern: String,
            // { numa_node_idx -> shm files count of this numa node }
            numa_node_files: HashMap<usize, usize>,
        }
        let cases = vec![
            TestCase {
                memory_file_dir_pattern: format!("abc/numa_{}", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 0), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/numa_{}", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 0), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/nu_{}_ma", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 0), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/nu_{}_ma/546", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 0), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/中{}文", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 0), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("/abc/123/nu_{}_ma/546/", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 4), (1, 2), (2, 3), (3, 2)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("///abc/123/nu_{}_ma/546///", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(0, 0), (1, 0), (2, 0), (3, 0)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/nu_{}_ma/546", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: vec![(3, 0)].into_iter().collect(),
            },
            TestCase {
                memory_file_dir_pattern: format!("abc/123/nu_{}_ma/546", NUMA_NODE_IDX_VAR_NAME),
                numa_node_files: Default::default(),
            },
        ];

        for c in cases {
            let tempdir = tempfile::tempdir().expect("Failed to create tempdir");

            let numa_node_num = *c.numa_node_files.keys().max().unwrap_or(&0) + 1;
            let mut expected_numa_memory_files = vec![vec![]; numa_node_num];
            for (numa_index, count) in c.numa_node_files {
                let dir = c
                    .memory_file_dir_pattern
                    .replacen(NUMA_NODE_IDX_VAR_NAME, numa_index.to_string().as_str(), 1);
                expected_numa_memory_files[numa_index] = generated_random_files(tempdir.path().join(dir.trim_matches('/')), count);
            }
            if expected_numa_memory_files.iter().all(Vec::is_empty) {
                expected_numa_memory_files = Vec::new();
            }

            let p = MemoryFileDirPattern::new(&c.memory_file_dir_pattern, tempdir.path().to_str().unwrap());
            let mut actually_numa_memory_files = scan_memory_files(&p).expect("scan memory files must be ok");
            actually_numa_memory_files.iter_mut().for_each(|files| files.sort());

            assert_eq!(expected_numa_memory_files, actually_numa_memory_files);
        }
    }

    fn generated_random_files(dir: impl AsRef<Path>, count: usize) -> Vec<PathBuf> {
        fn filename_fn(rng: &mut ThreadRng) -> String {
            let len = rng.gen_range(1..=30);
            repeat(()).map(|()| rng.sample(Alphanumeric)).map(char::from).take(len).collect()
        }

        let mut rng = rand::thread_rng();
        let dir = dir.as_ref();
        fs::create_dir_all(dir).expect("Failed to create dir");

        let mut files: Vec<PathBuf> = (0..count)
            .map(|_| {
                let filename = filename_fn(&mut rng);
                let p = dir.join(filename);
                let mut data = vec![0; rng.gen_range(0..100)];
                rng.fill(data.as_mut_slice());
                fs::write(&p, &data).expect("Failed to write random data");
                p
            })
            .collect();

        files.sort();
        files
    }
}
