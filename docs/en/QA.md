# damocles-cluster Q&A

## Q: What is the load state: key not found exception? Is it a key configuration issue?

**A**: `The load state: key not found` occurs during sector sealing or upgrading, and is caused by the state record corresponding to the sector not being found.

Here the `key not found` exception is originated from the underlying component, where `key` refers to the key in the kv database.

This exception usually occurs in the following scenarios:

1. The sector has already been terminated on the `damocles-manager` side through commands like `util sealer sectors abort`, but the corresponding `damocles-worker` is still executing tasks for that sector;
2. The `damocles-manager` has changed its `home` directory, resulting in inability to read previous sector record data;
3. The `damocles-worker` is connected to the wrong `damocles-manager` instance;

For 1), you can first use the `util sealer sectors list` command to observe the list of terminated sectors, confirm whether there is a record for the problematic sector, and restore it using `util sealer sectors restore` if it exists.

For other cases, configurations or connection information need to be corrected according to the actual situation.

## Q: Why is the compiled damocles-worker executable so large?

**A**: Here "very large" usually refers to an executable size up to GBs. Normally, the `damocles-worker` executable size is in the tens of MBs. A file in the GB range has definitely exceeded normal boundaries.

This situation usually occurs when debug information is accidentally enabled during compilation, with several common possibilities:

1. `[profile.\<name\>.debug]` is set in cargo config files at various levels;
2. Parameters that enable debug info are introduced in the compile command, these parameters may appear in the following locations:

- Environment variables: `XXXFLAG` environment variables like `RUSTFLAG`
- Compiler parameters: parameters like rustc's `-g`
- Compile configurations: configurations like `rustflags` in cargo config files at various levels

Regarding this issue, we noticed that the recommended environment variables in the official lotus INSTALL-linux documentation are:

export RUSTFLAGS="-C target-cpu=native -g"
 export FFI\_BUILD\_FROM\_SOURCE=1

Where `-C target-cpu=native` optimizes for the native CPU, while `-g` enables debug info.

If users follow `lotus` experience, they may encounter executable files that are particularly large. For such cases, we recommend users only configure:

export RUSTFLAGS="-C target-cpu=native"

Thanks to [caijian76](https://github.com/caijian76) from the community for providing feedback and clues.

## Q: What is the "Once instance has previously been poisoned" error? How to handle it?

**A**: Fundamentally, `Once` is a programming primitive type introduced for thread safety, often implemented using low-level techniques like OS locks. It is used widely across many occasions and libraries.

The `Once instance has previously been poisoned` exception occurs during system calls, and can have many causes. Here is one verified situation:

- When limiting cores for external processors, we use the **`** cgroup.cpuset` config
- When the work of the external processor is memory sensitive, like `pc1`, we normally also enable memory affinity i.e. `numa_preferred` config
- When the above configs are enabled together, and the cpu cores in `cgroup.cpuset` don't match the physical partition specified by `numa_preferred`, there is a high chance of this exception occurring.

Of course, the above situation may be just one of many possibilities. We will supplement here when discovering other cases.

Thanks to [steven](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U03C6L8RWP6) from the community for the feedback, and [Dennis Zou](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U01U2M1GZL7) for the explanation.

## Q: What is the `Too many open files (os error 24)` error? How to handle it?

**A**: "Too many open files (os error 24)" commonly occurs on Linux systems, indicating the current process has opened too many file handles.

This situation is more common during `WindowPoSt`, since each `WindowPoSt` task may need to read many sector files.

Generally, this issue can be resolved by increasing the file handle limit. But it's hard to determine a specific upper limit.

Therefore, in most cases, we just set the file handle limit to unlimited to avoid this problem.

The specific steps can refer to [[Tutorial] Permanently Setting Your ULIMIT System Value](https://github.com/filecoin-project/lotus/discussions/6198).

## Q: Why do I expect both GPUs to be utilized but actually one GPU stays idle all the time?

**A**: There are many possible causes for this issue, but generally speaking, users new to damocles-cluster often misconfigure some parameters due to misunderstandings about hardware and scheduling, resulting in this outcome. In other words, this is usually caused by misunderstandings of hardware and scheduling configurations.

Let's talk about the principles.

Generally, when configuring an external processor, we need to consider the actual situation in several aspects:

1. What hardware resources can this external processor utilize

This is mainly influenced by configurations like `cgroup.cpuset`, `numa_preferred`, and environment variables like `CUDA_VISIBLE_DEVICE`.

In other words, these are primarily system or startup level settings targeting the subprocess of this external processor.

2. Scheduling principles when using this external processor.

    1. Capability settings of the external processor itself.

        This is currently mainly the `concurrent` configuration item.

    2. Coordination between this external processor and other external processors.

        This situation is relatively complex and can be further divided into:

        - Coordination with other external processors in the same stage, such as `processors.limitation.concurrent,``processors.limitation.staggered`.
        - Coordination with external processors in different stages, such as `processors.ext_locks`.

Take this configuration on a machine with dual GPUs as an example:
```
[processors.ext_locks]
gpu1 = 1
gpu2 = 2


[[processors.pc2]]
cgroup.cpuset =  "2,5,8,11,14,17,20,23"
locks = ["gpu1"]
envs = { CUDA_VISIBLE_DEVICES = "0" }

[[processors.pc2]]
cgroup.cpuset =  "50,53,56,59,62,65,68,71"
locks = ["gpu2"]
envs = { CUDA_VISIBLE_DEVICES = "0" }

[[processors.c2]]
bin = /usr/local/bin/gpuproxy
args = ["args1", "args2", "args3"]
```

Here are a few common misunderstandings in this configuration:

1. The format of `ext_locks` is a user-defined lock, in the format `\<lock_name\> = \<number of external processors holding the lock simultaneously\>`. 
   
   In this case, `gpu2 = 2` is likely due to misunderstanding the meaning of the number.
2. `ext_locks` are commonly used when _external processors in different stages need exclusive use of the same hardware_, a common case being `pc2` and `c2` sharing one GPU.

    In this example, `c2` uses the GPU proxy instead of the local GPU, so the `ext_locks` setting for `pc2` has no effect.

3. Both `pc2` external processors have `CUDA_VISIBLE_DEVICE set to "0"`, which is the fundamental reason the second GPU stays idle. the two `pc2` processors can only see the GPU with index 0, so they are always using the same GPU.

    `CUDA_VISIBLE_DEVICE` is an environment variable provided in the official Nvidia driver. Its interpretation can be found in this tip: CUDA Pro Tip: Controlling GPU Visibility with CUDA\_VISIBLE\_DEVICES.

So the corrected configuration should be:
```
[[processors.pc2]]
cgroup.cpuset =  "2,5,8,11,14,17,20,23"
envs = { CUDA_VISIBLE_DEVICES = "0" }

[[processors.pc2]]
cgroup.cpuset =  "50,53,56,59,62,65,68,71"
envs = { CUDA_VISIBLE_DEVICES = "1" }

[[processors.c2]]
bin = /usr/local/bin/gpuproxy
args = ["args1", "args2", "args3"]
```


Thanks to [steven](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U03C6L8RWP6) from the community for providing the case.

## Q: What is the "memory map must have a non-zero length" error? How to handle it?

**A**: This error occurs when the target file size is 0 in the `mmap` process.

Based on experience, `mmap` is mainly used to read various static files in current Filecoin algorithms, such as:

- vk and params files
- Various pre-generated cache files like parent cache, etc.

So, this error usually indicates the target file does not exist or is an empty file.

For example, missing parameter or key files in a `WindowPoSt` scenario.

## Q: What is the `vc_processors::core::ext::producer: failed to unmarshal response string` error? How to handle it?

**A**: This error occurs because the external processor produced a response in the wrong format. You can enable/disable the dump feature in damocles-worker at runtime to view the wrongly formatted response:

Enable dump:
```
damocles-worker worker --config="path/to/your_config_file.toml" enable_dump --child_pid=<target_ext_processor_pid> --dump_dir="path/to/dump_dir"
```

Disable dump:
```
damocles-worker worker --config="path/to/your_config_file.toml" disable_dump --child_pid=<target_ext_processor_pid>
```

Debug based on the dump files.

## Q:
```
...storage_proofs_porep::stacked::vanilla::create_label::multi: create labels
thread '<unnamed>' panicked at 'assertion failed: `(left == right)`
  left: `0`,
 right: `2`, .../storage-proofs-porep-11.1.1/src/stacked/vanilla/cores.rs:151:5
```
What is the error? How to handle it?

**A**: This error is most likely caused by incorrect `cgroup.cpuset` configuration for PC1 in `damocles-worker`. See: Notes on cgroup.cpuset configuration.