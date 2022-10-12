# venus-cluster Q&A
## Q: load state: key not found 是什么异常？是密钥配置问题么？
**A**: `load state: key not found` 发生在扇区密封或升级过程中，是由于扇区对应的状态记录未找到导致的。

这里的 `key not found` 异常由底层的组件传导上来，其中的 `key` 是指 kv 数据库中的键。

这种异常通常发生在以下场景：
1. 已经在 `venus-sector-manager` 一侧通过类似 `util sealer sectors abort` 这样的命令终止了某个扇区，而对应的 `venus-worker` 仍在继续执行这个扇区的任务；
2. `venus-sector-manager` 更换了 `home` 目录，导致无法读取之前的扇区记录数据；
3. `venus-worker` 连接到了错误的 `venus-sector-manager` 实例；

对于 1)，可以先通过 `util sealer sectors list` 命令观察已终止的扇区列表，确认是否存在问题扇区对应的记录，如果存在的话，再通过 `util sealer sectors restore` 命令进行恢复。

对于其他情况，需要按照实际情况更正配置或连接信息。



## Q: 编译出的 venus-worker 可执行文件特别大，是什么原因？
**A**: 这里的特别大，通常是指可执行文件的体积达到上 G。正常来说，`venus-worker` 可执行文件的体积在数十M 这个量级。上 G 的文件肯定已经超出了正常的范畴。

这种情况通常是编译过程中意外启用了 debug 信息导致的，通常有几种可能性：
1. 在各层级的 [cargo config 文件](https://doc.rust-lang.org/cargo/reference/config.html) 中设置了 `[profile.<name>.debug]`；
2. 在编译指令中引入了启用 debug 信息的参数，这种参数可能出现在以下位置：
   - 环境变量：以 `RUSTFLAG` 为代表的各类 `XXXFLAG` 环境变量
   - 编译器参数：以rustc 的 `-g` 参数为代表的各类参数
   - 编译配置项：在各层级的 cargo config 文件中存在的、以 `rustflags` 为代表的各类配置项


关于这个问题，我们注意到，在 `lotus` 的官方文档 [INSTALL-linux](https://lotus.filecoin.io/lotus/install/linux/) 中提到的环境变量建议：
```
export RUSTFLAGS="-C target-cpu=native -g"
export FFI_BUILD_FROM_SOURCE=1
```

其中，`-C target-cpu=native` 的作用是针对本机 CPU 进行优化，而 `-g` 的作用就是启用 debug 信息。

如果用户按照 `lotus` 的经验，可能就会发现可执行文件体积特别大的情况。针对这种情况，我们推荐使用者仅配置
```
export RUSTFLAGS="-C target-cpu=native"
```

感谢来自社区的 [caijian76](https://github.com/caijian76) 提供反馈和线索。



## Q:  `Once instance has previously been poisoned` 是什么错误？如何处理？

**A**: 从最基本的原理来说，`Once` 是一类为了确保线程安全而产生的编程基础类型，它通常会使用到操作系统层面的锁等底层实现，被用于非常多的场合和类库中。

 出现 `Once instance has previously been poisoned` 这类异常是出现在系统调用中，它的原因可能有很多种。这里提出的只是其中一种已经被验证了的情况，描述如下：

- 当需要为外部处理器限核时，我们会使用 `cgroup.cpuset` 配置项
- 当外部处理器的工作内容是内存敏感类型，如 `pc1` 时，我们通常还会启用内存亲和性即 `numa_preferred ` 配置项
- 当上述配置同时启用，而 `cgroup.cpuset` 中的 cpu 核心不符合 `numa_preferred` 指定的物理分区时，有较高的可能性出现此类异常



当然，上面这种情况也许只是众多可能性中的一种。我们会在发现其他情况之后补充到这里来。

感谢来自社区的 [steven](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U03C6L8RWP6) 提供反馈，[Dennis Zou](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U01U2M1GZL7) 提供解答。



## Q: `Too many open files (os error 24)` 是什么错误？如何处理？

**A**：`Too many open files (os error 24)` 通常出现在 linux 系统中，表明当前进程开启了太多的文件句柄。

这种情况在 `WindowPoSt` 过程中较为常见，原因是每个 `WindowPoSt` 任务都有可能要读取大量的扇区文件。

通常来说，这种问题可以通过改变提高文件句柄限制来解决。但是我们很难确定一个具体的上限值。

因此，在大多数场景中，我们直接将文件句柄限制设置为无限来规避这种问题。

具体操作方式可以参考 [[Tutorial] Permanently Setting Your ULIMIT System Value](https://github.com/filecoin-project/lotus/discussions/6198)。



## Q：为什么我期望的结果是两张 GPU 都被使用，但是实际情况有一张 GPU 始终空闲？

**A**：发生这种问题的原因有很多，但是一般来说，刚接触 venus-cluster 的用户在进行资源编排的时候误理解和配置了一些参数导致这种结果的情况比较多。换句话说，这通常是由*对硬件和调度配置的误解* 导致的。

这里我们从原理说起。

一般来说，配置一个外部处理器的时候，我们需要考虑几个方面的实际情况：

1. 这个外部处理器能使用哪些硬件资源

   这一块主要受诸如 `cgroup.cpuset` 、`numa_preferred` 以及环境变量 `CUDA_VISIBLE_DEVICE` 等影响。

   换句话说，这些主要是针对这一个外部处理器子进程的、系统或启动级别的设置。

2. 使用这个外部处理器时的调度原则

   1. 这个外部处理器自身的处理能力设定

      这一块目前主要是 `concurrent` 配置项

   2. 这个外部处理器和其他外部处理器的协调情况

      这种情况相对复杂，还可以再细分为：

      - 和其他相同阶段的外部处理器协调

        如 `processors.limitation.concurrent`、`processors.limitation.staggered`

      - 和其他不同阶段的外部处理器协调

        如 `processors.ext_locks`



以题目中的、在一台双 GPU 的机器上实行的这样一份配置为例：

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

其中存在这样几点常见误解：

1. 关于 `ext_locks` 的配置格式，它是一个完全由用户自行定义的锁，其格式为 `<锁名> = <同时持锁的外部处理器数量>`。

   在这个场景下，`gpu2 = 2` 很有可能来自于对数字含义的误解。

2. `ext_locks` 通常用在 *不同阶段的处理器需要独占使用同一个硬件* 的场景，比较常见的是 `pc2` 和 `c2` 共用一块 GPU。

   在这个范例中，`c2` 使用了代理 GPU 的方案，不使用本地 GPU，因此 `pc2` 的 `ext_locks` 设置并无效果。

3. 两个 `pc2` 的外部处理器都设定了 `CUDA_VISIBLE_DEVICE = "0"`，这是第二块 GPU 空闲的根本原因，即两个 `pc2` 外部处理器都只能看到序号为 0 的 GPU，也就是说他们始终在使用同一块 GPU。

   `CUDA_VISIBLE_DEVICE` 是 nvidia 官方驱动中提供的一个环境变量，它的解读可以参考 [CUDA Pro 技巧：使用 CUDA_VISIBLE_DEVICES 控制 GPU 的可见性](https://developer.nvidia.com/zh-cn/blog/cuda-pro-tip-control-gpu-visibility-cuda_visible_devices/)



那么经过修正后的配置应当为：

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



感谢社区的 [steven](https://app.slack.com/client/TEHTVS1L6/C028PCH8L31/user_profile/U03C6L8RWP6) 提供案例。



## Q：`memory map must have a non-zero length` 是什么错误？如何处理？

**A**：这种报错信息表达的是，是使用 `mmap` 的过程中，目标文件大小为 0。

根据经验，在目前 Filecoin 的各类算法中， `mmap` 的使用主要出现在各类静态文件的读取，如：

- vk 和 params 文件
- 各类事先生成的 cache 文件，如 parent cache 等

那么出现这种错误，通常表示目标文件不存在，或为空文件。

例如，在 `WindowPoSt` 场景，如果没有准备好参数文件或密钥文件。

## Q: `vc_processors::core::ext::producer: failed to unmarshal response string` 是什么错误？ 如何处理？

**A**：发生这种错误是因为外部处理器产生了错误格式的响应。可以通过 venus-worker 提供的命令运行时的开启或关闭 dump 功能，查看错误格式的响应。

开启 dump
```
venus-worker worker --config="path/to/your_config_file.toml" enable_dump --child_pid=<target_ext_processor_pid> --dump_dir="path/to/dump_dir"
```

关闭 dump
```
venus-worker worker --config="path/to/your_config_file.toml" disable_dump --child_pid=<target_ext_processor_pid>
```

可根据 dump 文件进行 debug。

## Q: 
```
...storage_proofs_porep::stacked::vanilla::create_label::multi: create labels
thread '<unnamed>' panicked at 'assertion failed: `(left == right)`
  left: `0`,
 right: `2`, .../storage-proofs-porep-11.1.1/src/stacked/vanilla/cores.rs:151:5
```
是什么错误？ 如何处理？

**A**: 这个错误的原因是 venus-worker 中 PC1 的 `cgroup.cpuset` 配置问题引发的. 参考: [`cgroup.cpuset` 配置注意事项](./03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#cgroupcpuset-%E9%85%8D%E7%BD%AE%E6%B3%A8%E6%84%8F%E4%BA%8B%E9%A1%B9)
