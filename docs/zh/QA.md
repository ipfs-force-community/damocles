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
