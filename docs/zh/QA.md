# venus-cluster Q&A
## Q: load state: key not found 是什么异常？是密钥配置问题么？
A: `load state: key not found` 发生在扇区密封或升级过程中，是由于扇区对应的状态记录未找到导致的。

这里的 `key not found` 异常由底层的组件传导上来，其中的 `key` 是指 kv 数据库中的键。

这种异常通常发生在以下场景：
1. 已经在 `venus-sector-manager` 一侧通过类似 `util sealer sectors abort` 这样的命令终止了某个扇区，而对应的 `venus-worker` 仍在继续执行这个扇区的任务；
2. `venus-sector-manager` 更换了 `home` 目录，导致无法读取之前的扇区记录数据；
3. `venus-worker` 连接到了错误的 `venus-sector-manager` 实例；

对于 1)，可以先通过 `util sealer sectors list` 命令观察已终止的扇区列表，确认是否存在问题扇区对应的记录，如果存在的话，再通过 `util sealer sectors restore` 命令进行恢复。

对于其他情况，需要按照实际情况更正配置或连接信息。

## Q: 编译出的 venus-worker 可执行文件特别大，是什么原因？
A: 这里的特别大，通常是指可执行文件的体积达到上 G。正常来说，`venus-worker` 可执行文件的体积在数十M 这个量级。上 G 的文件肯定已经超出了正常的范畴。

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
