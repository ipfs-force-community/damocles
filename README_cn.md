## venus-cluster
`venus-cluster` 是一套 `Filecoin` 算力集群方案，专注于算力积累和维持。相比 `lotus` 的默认方案，它在以下几方面进行更多尝试：
- 简化 sealing 状态机
- 通过改造流程来提升资源的使用效率
- 基于 `venus` 及相关组件提供的链、消息服务，提供丰富的本地策略，以期获得更好的经济性表现
- 多租户的支持与协作

`venus-cluster` 由 `venus-worker` 和 `venus-sector-manager`  两个组件组成，前者用于进行 `PoRep` 的计算过程，后者负责扇区的管理，和所有与链交互的过程。

### 准备

#### 环境
对于编译环境的准备，`venus-cluster` 与 `lotus` 没有太多的差别，可以参考 [Basic Build Instructions](https://github.com/filecoin-project/lotus#basic-build-instructions)。


#### 编译
通过以下命令完成组件的编译：
```
make build-smgr
make build-worker
```
编译结果会被复制到 `./dist/bin/` 文件夹内。


### 使用
在本段中，将以基于文件目录的数据同步方式为例，介绍 `venus-cluster` 的使用方式。
对于依赖于 `venus` 和 `venus-messager`，的部分，如 `venus-wallet` 等，需要按照要求进行配置，这里不再赘述。

#### venus-sector-manager
`venus-sector-manager` 的配置主要集中在链服务、消息以及消息上链的策略等。

##### 全局 flag
目前，`venus-sector-manager` 具备两个全局 `flag`， 分别是：
- home：数据目录，用来保存本地数据，默认位于 `~/.venus-sector-manager`
- net：用于选择加入的 `Filecoin` 网络，默认 `mainnet`

##### 初始化
使用者需要使用以下命令进行初始化：
```
venus-sector-manager daemon init
```
这将会初始化 `home` 目录下的内容，如配置文件、元数据库等。


##### 准备配置文件
完成初始化之后的配置文件内容如下：
```
# Default config:
[SectorManager]
#  Miners = []
#  PreFetch = true
#
[Commitment]
#  [Commitment.DefaultPolicy]
#    CommitBatchThreshold = 0
#    CommitBatchMaxWait = "0s"
#    CommitCheckInterval = "0s"
#    EnableBatchProCommit = false
#    PreCommitBatchThreshold = 0
#    PreCommitBatchMaxWait = "0s"
#    PreCommitCheckInterval = "0s"
#    EnableBatchPreCommit = false
#    PreCommitGasOverEstimation = 0.0
#    ProCommitGasOverEstimation = 0.0
#    BatchPreCommitGasOverEstimation = 0.0
#    BatchProCommitGasOverEstimation = 0.0
#    MaxPreCommitFeeCap = ""
#    MaxProCommitFeeCap = ""
#    MaxBatchPreCommitFeeCap = ""
#    MaxBatchProCommitFeeCap = ""
#    MsgConfidence = 0
#  [Commitment.Miners]
#    [Commitment.Miners.example]
#      [Commitment.Miners.example.Controls]
#        PreCommit = ""
#        ProveCommit = ""
#
[Chain]
#  Api = ""
#  Token = ""
#
[Messager]
#  Api = ""
#  Token = ""
#
[PersistedStore]
#  Includes = []
#  Stores = []
#
[PoSt]
#  [PoSt.Default]
#    StrictCheck = false
#    GasOverEstimation = 0.0
#    MaxFeeCap = ""
#    MsgCheckInteval = "1m0s"
#    MsgConfidence = 5
#  [PoSt.Actors]
#
```

如果使用者希望以最简方式启动，那么只需要填写部分内容，以使用 MinerID `f012345` 为例，内容如下：
```
[SectorManager]
# 允许进行扇区管理的 MinerID
Miners = [
        12345
]

# 启动时预取矿工信息
PreFetch = true

[Commitment]
[Commitment.Miners]

# 针对 f012345 的复制证明提交配置段
[Commitment.Miners.12345]

# 针对 f012345 的复制证明提交所使用的钱包地址配置段
[Commitment.Miners.12345.Controls]

# 用于扇区 PreCommit 消息发送的钱包地址
PreCommit = "f3vur6njy6uucabdgxujlq2tsvtfrhbjwyhumangldl5mbsuzm4otzrsoykmejpukkiywmqidhksbvm32qnjga"

# 用于扇区 ProveCommit 消息发送的钱包地址
ProveCommit = "f3vur6njy6uucabdgxujlq2tsvtfrhbjwyhumangldl5mbsuzm4otzrsoykmejpukkiywmqidhksbvm32qnjga"

# 链服务连接信息
[Chain]
Api = "/ip4/127.0.0.1/tcp/3453"
Token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoidC12ZW51cy1zZWFsZXIiLCJwZXJtIjoic2lnbiIsImV4dCI6IiJ9.4Ja5hqVQAshewiYzRTxKxWE8gLwRC-R21MJPLNeqLu0"

# 消息服务连接信息
[Messager]
Api = "/ip4/127.0.0.1/tcp/39812"
Token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoidC12ZW51cy1zZWFsZXIiLCJwZXJtIjoic2lnbiIsImV4dCI6IiJ9.4Ja5hqVQAshewiYzRTxKxWE8gLwRC-R21MJPLNeqLu0"

# 时空证明配置段
[PoSt]

# 针对 f012345 的时空证明配置段
[PoSt.Actors.12345]

# 用于 WindowPoSt 消息发送的钱包地址
Sender = "f3vur6njy6uucabdgxujlq2tsvtfrhbjwyhumangldl5mbsuzm4otzrsoykmejpukkiywmqidhksbvm32qnjga"
```


##### 准备矿工号
使用 `venus-sector-manager` 首先需要具备 `MinerID`，如上文中的 `f012345`。
如果没有通过其他渠道申请，那么可以考虑使用 `venus-sector-manager` 提供的命令行工具，方式可以参考：

```
$ ./dist/bin/venus-sector-manager util miner create --help
NAME:
   venus-sector-manager util miner create -

USAGE:
   venus-sector-manager util miner create [command options] [arguments...]

OPTIONS:
   --from CreateMiner   Wallet address used for sending the CreateMiner message
   --owner Owner        Actor address used as the Owner field. Will use the AccountActor ID of `from` if not provided
   --worker Worker      Actor address used as the Worker field. Will use the AccountActor ID of `from` if not provided
   --sector-size value  Sector size of the miner, 512MiB, 32GiB, 64GiB, etc
   --peer value         P2P peer id of the miner
   --multiaddr value    P2P peer address of the miner
   --help, -h           show help (default: false)

```
注意：要使用链相关的命令行工具，需确保配置文件中的 `Chain` 和 `Messager` 两个段配置完整。

##### 启动
在完成配置文件的填写后，使用形似以下格式的命令启动：
```
./dist/bin/venus-sector-manager --net=nerpa daemon run --poster
```
以这条命令为例，将启动在 `nerpa` 网络上，并且启用时空证明模块。


#### venus-worker
`venus-worker` 的配置主要集中在资源的分配和使用方式。


##### 准备 cgroup
由于 `venus-worker` 允许使用者启动子进程形态的 `sealing` 单步处理器，并通过 `cgroup` 对这些子进程进行资源管理，因此需要通过执行以下命令获得 `cgroup` 的相关操作权限：
```
./venus-worker/create-cgroup.sh
```
注意：此脚本将为特定的 `cgroup` 资源组分配当前用户的执行权限。这也就意味着，如果使用者打算用特定的用户启动 `venus-cluster`，需要先切换到目标用户再执行脚本。



##### 准备本地目录
本地目录用于保存 `PoRep` 过程中产生的文件。
`venus-worker` 的设计理念是*按需配置*， 即配置与计算资源相匹配的本地存储资源，以让所有活跃中的扇区能够相互独立、并行不悖。

使用者通过以下方式来初始化将要使用的本地目录：
```
./dist/bin/venus-worker store seal-init -l path/to/local/store1 path/to/local/store2 path/to/local/store3
```

##### 配置持久化目录
持久化目录用于保存 `PoRep` 的结果文件。
常见的做法是将存储服务器的对应目录挂载到本地。为了避免在丢失挂载点的情况下启动，也需要对持久化目录进行初始化。
具体做法是在成功挂载之后，通过以下方式来初始化持久化目录：
```
./dist/bin/venus-worker store file-init -l path/to/remote/store
```


##### 准备配置文件
配置文件使用 `toml` 格式。

```
# venus-sector-manager 的服务地址
[sealer_rpc]
url = "ws://127.0.0.1:1789/rpc/v0"

# sealing 过程配置
[sealing]

# 是否接受扇区内包含订单
enable_deals = false

# 对于 sealing 过程中的临时性异常，允许自动重试的次数
max_retries = 3

# 本地目录配置，按照实际情况配置，需初始化
[[store]]
location = "path/to/local/store1"

[[store]]
location = "path/to/local/store2"

[[store]]
location = "path/to/local/store3"

# 各阶段并行任务数量配置
[limit]

# pc1 阶段的并发数量控制
# 仅在一些特定的场景下进行配置，如计算资源少于按照存储资源所需的数量
# pc1 = 1

pc2 = 1
c2 = 1

# 持久化目录配置，按照实际情况配置，需初始化
[remote]
path = "path/to/remote/store"

# pc2 子进程处理器相关配置
[processors.pc2]

# 启动子进程处理器
external = true

# c2 子进程处理器相关配置
[cessors.c2]

# 启动子进程处理器
external = true

# 子进程的 cgroup 资源控制配置
[processors.c2.cgroup]

# 子进程可使用的处理器核心
# cpuset = "24-47"
```

##### 启动
在完成配置文件的填写后，使用以下命令启动：
```
./dist/bin/venus-worker daemon -c path/to/venus-worker.toml
```

为了显示相应级别的日志，可以在启动前配置 `RUST_LOG` 环境变量，如 `RUST_LOG=debug`。
