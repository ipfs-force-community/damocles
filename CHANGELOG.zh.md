# Changelog

## v0.6.6
- venus-sector-manager
  - 修复 snapdeal 判断目标 CC 扇区是否为时空证明锁定期的 bug [#789](https://github.com/ipfs-force-community/damocles/pull/789)
  - snapdeal 支持配置不清除原 CC 扇区数据。新增配置 `Miners.SnapUp.CleanupCCData` [#790](https://github.com/ipfs-force-community/damocles/pull/790)


## v0.6.5
- docker
  - 安装 tzdata 解决日志时间的时区问题，现在启动容器时增加环境变量 TZ 即可设置时区 (例如: TZ=Asia/Shanghai)。 [#752](https://github.com/ipfs-force-community/venus-cluster/pull/752)
- venus-sector-manager
  - 增加日志输出，window post 消息成功上链会打印形如 `window post message succeeded: xxx` (`xxx` 为消息签名后的 cid) 的日志。[#755](https://github.com/ipfs-force-community/venus-cluster/pull/755)
  - 修复 snapup 重试机制与 deadline 锁定期冲突的 bug [#744](https://github.com/ipfs-force-community/venus-cluster/pull/744)


## v0.6.4
- docker
  - 修复 opencl 驱动安装 [#750](https://github.com/ipfs-force-community/venus-cluster/pull/750)

## v0.6.3
- venus-sector-manager
  - 提交 precommit 消息时，当 CommD 与链上不一致时，不再直接 abort 扇区，而是暂停封装线程等待人为介入处理 [#742](https://github.com/ipfs-force-community/venus-cluster/pull/742)
  - 提交 precommit 消息时，如果 precommit 消息执行失败不再直接 abort 扇区，而是暂停封装线程等待人为介入处理 [#745](https://github.com/ipfs-force-community/venus-cluster/issues/745)

## v0.6.2
- venus-sector-manager
  - 调用新 api `ReleaseDeals` 释放订单避免重复释放订单 bug [#664](https://github.com/ipfs-force-community/venus-cluster/pull/664)
  - 修改部分日志的日志级别 debug -> info [#738](https://github.com/ipfs-force-community/venus-cluster/pull/738)

## v0.6.1
- venus-sector-manager
  - 日志默认级别从 `DEBUG` 修改为 `INFO` [#719](https://github.com/ipfs-force-community/venus-cluster/pull/719)
  - 修复 `ProveReplicaUpdates` 消息可能在 windowPoST 窗口锁定期提交的 bug [#494](https://github.com/ipfs-force-community/venus-cluster/pull/494)
  - cli: `util sealer sectors extend` 使用新版续期方法 `ExpirationExtension2` [#636](https://github.com/ipfs-force-community/venus-cluster/pull/636)
  - 支持从环境变量设置数据和配置目录 [#733](https://github.com/ipfs-force-community/venus-cluster/pull/733)

## v0.6.0
- venus-sector-manager
  - 升级依赖 [#710](https://github.com/ipfs-force-community/venus-cluster/pull/710)

## v0.6.0-rc3
- venus-sector-manager
  - ext-prover 模式的 wdpost 兼容 rust-filecoin-proofs-api 版本类型 [#702](https://github.com/ipfs-force-community/venus-cluster/pull/702)

## v0.6.0-rc2
- venus-sector-manager
  - 适配支持 nv19 (增加 WindowPoSt 证明类型 [#685](https://github.com/ipfs-force-community/venus-cluster/pull/685); [FIP 0061](https://github.com/filecoin-project/FIPs/blob/master/FIPS/fip-0061.md))

- venus-worker：
  - 升级 rust-fil-proofs 到 v14.0.0 [#687](https://github.com/ipfs-force-community/venus-cluster/pull/687)
  - 升级 rust-toolchain 到 1.67.1
  - 修复 rpc 地址配置域名时的 bug。 [#691](https://github.com/ipfs-force-community/venus-cluster/pull/691)

  venus-worker 升级说明: 在 venus-sector-manager 中使用了 [ext-prover 执行器](https://github.com/ipfs-force-community/venus-cluster/blob/release/v0.6/docs/zh/09.%E7%8B%AC%E7%AB%8B%E8%BF%90%E8%A1%8C%E7%9A%84poster%E8%8A%82%E7%82%B9.md#ext-prover-%E6%89%A7%E8%A1%8C%E5%99%A8) 做 WindowPoSt 的用户需要升级 venus-worker 到此版本。否则可以选择不升级 venus-worker


## v0.6.0-rc1
- venus-sector-manager
  - 插件支持自定义数据库。 [#561](https://github.com/ipfs-force-community/venus-cluster/issues/561)
  - 插件支持自定义注册 jsonrpc 接口。 [#595](https://github.com/ipfs-force-community/venus-cluster/issues/595)
  - daemon 移除 `--net` flag, 自动获取网络参数 [#574](https://github.com/ipfs-force-community/venus-cluster/pull/574)
  - 禁用 `util sealer sectors abort` abort 扇区命令 [#660](https://github.com/ipfs-force-community/venus-cluster/pull/660)
  - cli `util miner info` 支持打印受益人地址 [#418](https://github.com/ipfs-force-community/venus-cluster/issues/418)
  - cli `util sealer actor withdraw` 新增 `--beneficiary` flag 用于支持受益人体现。 [#546](https://github.com/ipfs-force-community/venus-cluster/pull/546)
  - cli 新增 `util sealer actor propose-change-beneficiary` 和 `util sealer actor confirm-change-beneficiary` 命令用于支持增加收益人。 [#546](https://github.com/ipfs-force-community/venus-cluster/pull/546)
  - 支持手动发送 recover 消息。 [#382](https://github.com/ipfs-force-community/venus-cluster/issues/382)
  - 支持手动设置扇区状态为 finalize。 [#657](https://github.com/ipfs-force-community/venus-cluster/pull/657)
  - cli 新增移除过期 worker 的命令。 [#493](https://github.com/ipfs-force-community/venus-cluster/issues/493)
  - 支持 lotus-miner 与 vsm 相互切换。[参考文档](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/17.%20venus-cluster%E4%B8%8Elotus-miner%E5%88%87%E6%8D%A2%E6%B5%81%E7%A8%8B.md)。 [#625](https://github.com/ipfs-force-community/venus-cluster/issues/625)
  - cli `util sealer proving` 输出信息调整优化。 [#568](https://github.com/ipfs-force-community/venus-cluster/issues/568)
  - wdpost 扇区检查并发与超时设置。[参考文档](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonproving)。 [#532](https://github.com/ipfs-force-community/venus-cluster/issues/532)
  - 修改 submitpost 的逻辑，变成做完即发送的模式。方便监控和处理 windowpost过程中的意外情况。[#590](https://github.com/ipfs-force-community/venus-cluster/issues/590)
  - cli 合并 `util sealer sectors extend` 和 `util sealer sectors renew` 命令为 `util sealer sectors extend`，新增 `--max-sectors` flag 用于控制每条 extend 消息中包含的扇区数量上限，新增 `--only-cc` flag 用于控制是否只扩展 cc 扇区。 [#582](https://github.com/ipfs-force-community/venus-cluster/pull/582)

- venus-worker
  - 外部执行器子进程意外退出支持自动重启，[参考配置文档](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#processorsstage_name) (processors.{stage_name}.auto_restart) [#605](https://github.com/ipfs-force-community/venus-cluster/pull/605)
  - vsm 的 rpc 地址支持配置域名。 [#661](https://github.com/ipfs-force-community/venus-cluster/pull/661)
  - 升级算法库 rust-fil-proofs 到 v12.0。 [#490](https://github.com/ipfs-force-community/venus-cluster/issues/490)
  - 升级 rust-toolchain 1.60.0 -> 1.67.1。[#380](https://github.com/ipfs-force-community/venus-cluster/issues/380)

- 其他
  - 新增 Dockerfile。[#659](https://github.com/ipfs-force-community/venus-cluster/pull/659)
  - 已合并到 v0.4 和 v0.5 的 bug 修复
    - 消息聚合 bug 修复 [#639](https://github.com/ipfs-force-community/venus-cluster/issues/639)
    - 修复订单释放 bug [#602](https://github.com/ipfs-force-community/venus-cluster/issues/602)
    - 修改 cli list sector 中对于不存在 laststate 的结构的扇区不输出其他信息的 bug [#551](https://github.com/ipfs-force-community/venus-cluster/pull/551)
    - 修改 snapup 对于消息处理和可重试错误的逻辑，在遇到 outofgas 或者钱不足等问题时自己进行重试 [#545](https://github.com/ipfs-force-community/venus-cluster/pull/545)
    - 修复 mongodb 删除数据的 bug [#548](https://github.com/ipfs-force-community/venus-cluster/pull/548)
    - 修复 WindowPoS 无法识别部分错误扇区的 bug [#535](https://github.com/ipfs-force-community/venus-cluster/issues/535)
    - 修复 Terminate Sector 消息 failed 不能正确退出监听的问题 [#507](https://github.com/ipfs-force-community/venus-cluster/issues/507)
    - 修复在开启消息聚合消息，同时关闭从ctrl地址发送质押，生成消息时，没有包含扇区的问题 [#510](https://github.com/ipfs-force-community/venus-cluster/issues/510)
    - 修复 SnapUp 没有正确处理 errormsg 的问题 [#524](https://github.com/ipfs-force-community/venus-cluster/issues/524)
    - 修复 venus-worker 配置 enable_deals=false 时触发的 bug [#501](https://github.com/ipfs-force-community/venus-cluster/issues/501)

## v0.5.0
- venus-sector-manager
  - MongoDB 数据库支持。[文档](./docs/zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonmongokvstore) [#323](https://github.com/ipfs-force-community/venus-cluster/issues/323)
  - metrics 支持。 [文档](./docs/zh/14.venus-sector-manager%E7%9A%84mertics%E4%BD%BF%E7%94%A8.md) [[#339](https://github.com/ipfs-force-community/venus-cluster/issues/339)]

  - Poster 改造:
    - Poster 重构，支持多 miner、多 deadline、多 partition 并行 [[#136](https://github.com/ipfs-force-community/venus-cluster/issues/136)]
    - 支持配置 `GasOverPremium`, `feecap` 和 `maxfee`. 具体参考[配置文档](./docs/zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md) [[#337](https://github.com/ipfs-force-community/venus-cluster/pull/337)]
    - 限制单条消息 recover 扇区的数量   增加 `MaxRecoverSectorLimit` 配置项， 参考[配置文档](./docs/zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md) [[#364](https://github.com/ipfs-force-community/venus-cluster/issues/364)]
  - 支持本地 solo 模式的 market [[#357](https://github.com/ipfs-force-community/venus-cluster/pull/357)], [[#361](https://github.com/ipfs-force-community/venus-cluster/pull/361)]

  - CLI 相关:
    - proving deadline 相关显示优化 [[#365](https://github.com/ipfs-force-community/venus-cluster/issues/365)]
    - 原内嵌数据库 badger 中数据迁移到 mongodb 的工具支持。`venus-sector-manager util migrate-badger-mongo`
    - 支持验证扇区文件内容 `venus-sector-manager util sealer proving --miner <miner_id> check --slow <deadlineIdx>` [[#430](https://github.com/ipfs-force-community/venus-cluster/issues/430)]
    - sealing_thread 列表新增 plan 列。 `venus-sector-manager util worker info <worker instance name or address>`，显示 sealing_thread 的 plan 信息 [[#428](https://github.com/ipfs-force-community/venus-cluster/issues/428)]
    - 增加从 lotus-miner 导入扇区详细数据的工具 `venus-sector-manager util sealer sectors import` [[#327](https://github.com/ipfs-force-community/venus-cluster/pull/327)]
  - 杂项:
    - 跟踪消息上链逻辑增加状态为 ReplaceMsg 的消息处理 [[#435](https://github.com/ipfs-force-community/venus-cluster/issues/435)]
    - 引入 venus v1.6.1; 引入 filecoin-ffi 内存泄漏修复后的版本; 引入 lotus v1.17.0 并进行兼容性修复 [#331](https://github.com/ipfs-force-community/venus-cluster/issues/331)

- venus-worker
  - 支持扇区重建功能。[文档](./docs/zh/16.%E6%89%87%E5%8C%BA%E9%87%8D%E5%BB%BA%E7%9A%84%E6%94%AF%E6%8C%81.md) [[#362](https://github.com/ipfs-force-community/venus-cluster/issues/362)]
  - `sealing_thread` 配置热更新支持。[文档](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#sealing_thread-%E9%85%8D%E7%BD%AE%E7%83%AD%E6%9B%B4%E6%96%B0)

  - 优化 add_piece, 包括：
    - 新增 add_pieces 外部执行器。 目的是为了进行并发控制, 避免所有的 add_pieces 同时启动，导致内存不足。[配置文档](./docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#processorsstage_name)(`[[processors.add_pieces]]` & `processors.limitation.concurrent.add_pieces`) [[#403](https://github.com/ipfs-force-community/venus-cluster/issues/403)]
    - add piece 算法优化, 大幅减少 add 超大 Piece 文件的内存使用。[[#415](https://github.com/ipfs-force-community/venus-cluster/issues/415)]
    - worker 支持加载本地 piece 文件。[配置文档](./docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#worker)(`worker.local_pieces_dir` 配置项) [[#444](https://github.com/ipfs-force-community/venus-cluster/issues/444)]

  - PC1 大页内存支持。[文档](./docs/zh/15.venus-worker_PC1_HugeTLB_Pages_%E6%94%AF%E6%8C%81.md)
  - CLI 相关:
    - sealing_thread 列表新增 plan 列。 `venus-worker worker list`，显示 sealing_thread 的 plan 信息 [[#428](https://github.com/ipfs-force-community/venus-cluster/issues/428)]
  - 杂项：
    - vc-processors 优化日志输出。[[#348](https://github.com/ipfs-force-community/venus-cluster/issues/348)]
    - 移除过大尺寸的日志。[[#417](https://github.com/ipfs-force-community/venus-cluster/pull/417)]
    - 修复 `venus-worker-util hwinfo` 报错 segmentation fault. [[#341](https://github.com/ipfs-force-community/venus-cluster/issues/341)]
    - 减少不必要的数据库写入操作

## v0.4.0-rc1

- venus-sector-manager
  - 支持为 `SP` 设置最小扇区序号，并可以实时生效，用以替代仅能在第一次分配时生效的 `InitNumber` 配置项
  - 修复使用 `bufio.Scanner` 带来的，在交互数据量较大时，无法正常与外部处理器通信的问题
  - 启用 `jsonrpc` 客户端的自动重试机制
  - 修复 `util miner info` 中 `Multiaddr` 显式乱码的问题
  - 修复重复执行 `daemon init` 覆盖已存在的配置文件的问题
  - 增加 winning post 的预热功能
  - 优化 SnapUp 任务流程，包括：
    - 支持根据候选扇区生命周期筛选订单（需要 venus-market 相应版本支持）
    - 改善终止 SnapUp 任务时的清理
    - 完善 SnapUp 任务完成时的旧扇区数据清理
  - 优化封装任务流程，包括：
    - 完善对于无法获取扇区信息记录的场景的处理
    - 改善手动 Abort 的扇区任务的处理
    - 针对 SysErrOutOfGas 类信息特殊处理
  - 重构持久化存储管理，包括：
    - 将持久化存储分配和管理逻辑从 venus-worker 上剥离，统一集中到 venus-sector-manager 上
    - 通过 golang plugin 的方式支持自定义持久化存储管理
    - 存储实例分配支持根据 MinerID 白名单/黑名单执行相应策略
  - 修复外部导入的扇区无法 terminate 的问题
  - 支持使用外部 `winning post` 处理器
  - CLI 工具相关：
    - 调整 扇区列表 子命令，支持输出不同任务类型、活跃和非活跃数据、根据 MinerID 过滤等
    - 增加用于输出指定扇区的全量信息的子命令
    - 增加查询订单所属扇区的子命令
    - 增加用于重发 pre / prove 上链信息的子命令
  - 配置调整：
    - 增加 `[[Common.PersistStores]]` 中的 `AllowMiners` 和 `DenyMiners` 配置项
    - 增加 `[[Common.PersistStores]]` 中的 `Meta` 配置项
    - 增加 `[[Common.PersistStores]]` 中的 `Plugin` 配置项
    - 增加 `[[Common.PersistStores]]` 中的 `ReadOnly` 配置项
    - 增加 `[[Common.PersistStores]]` 中的 `Weight` 配置项
    - 增加 `[[Common.PieceStores]]` 中的 `Meta` 配置项
    - 增加 `[[Common.PieceStores]]` 中的 `Plugin` 配置项
    - 增加 `[[Miners.Sector]]` 中的 `MinNumber` 配置项



- venus-worker
  - 将外部处理器相关代码剥离形成独立的公开库，以方便集成第三方定制算法
  - 增加 PoSt 相关的外部处理器类型，供 venus-sector-manager 使用
  - 增加数据传输相关的外部处理器类型，以方便集成非文件系统存储方案
  - 修复 `cgroup` 配置生命周期异常的问题
  - 支持将与外部处理器交互的数据 dump 成文件，方便 debug
  - 引入对应 filecoins-proofs v11 系列版本对应的自定义封装算法，支持 pc1 多核模式下，CPU核与预分配内存基于 numa 区域强绑定，以提升时间表现稳定性
  - 配置调整：
    - `[[processors.{stage_name}]]` 新增 `transfer` 阶段，配置用于数据传输的外部处理器相关
- 工具链
  - 支持在 macOS 上编译
  - 提供本机硬件嗅探工具
  - 提供封装生产循环计算器
  - 提供生成预分配在指定 numa 区域的整块内存文件的工具
- 文档
  - 新增关于 venus-worker-util 工具的使用文档
  - 新增关于自定义算法和存储方法的概述文档
- 其他改善和修复

详细的 PRs 和 Issues 可以参考 [venus-cluster milestone v0.4.0](https://github.com/ipfs-force-community/venus-cluster/milestone/4?closed=1)。



## v0.3.1
- venus-sector-manager：
  - 支持用于调节 PoSt 环节消息发送策略的 `MaxPartitionsPerPoStMessage` 和 `MaxPartitionsPerRecoveryMessage` 配置项

## v0.3.0

- venus-sector-manager：
  - 适配和支持 nv16
  - 对于一些特定类型的异常，返回特殊结果，方便 venus-worker 处理：
    - c2 消息上链成功，但扇区未上链 [#88](https://github.com/ipfs-force-community/venus-cluster/issues/88)
    - 对于能够确定感知到 ticket expired 的场景，直接终止当前扇区 [#143](https://github.com/ipfs-force-community/venus-cluster/issues/143)
    - 对于通过 `venus-sector-manager` 终止的扇区，或因其他原因缺失扇区状态信息的情况，直接终止当前扇区 [#89](https://github.com/ipfs-force-community/venus-cluster/issues/89)
  - 升级 `go-jsonrpc` 依赖，使之可以支持部分网络异常下的重连 [#97](https://github.com/ipfs-force-community/venus-cluster/issues/97)
  - 支持新的可配置策略：
    - 各阶段是否随消息发送 funding [#122](https://github.com/ipfs-force-community/venus-cluster/issues/122)
    - SnapUp 提交重试次数 [#123](https://github.com/ipfs-force-community/venus-cluster/issues/123)
  - 支持可配置的 `WindowPoSt Challenge Confidential` [#163](https://github.com/ipfs-force-community/venus-cluster/issues/163)
  - 迁入更多管理命令
  - 配置调整：
    - 新增 [Miners.Commitment.Terminate] 配置块
    - 新增 [Miners.SnapUp.Retry] 配置块
    - 新增 [Miners.SnapUp] 中的 SendFund 配置项
    - 新增 [Miners.Commitment.Pre] 中的 SendFund 配置项
    - 新增 [Miners.Commitment.Prove] 中的 SendFund 配置项
    - 新增 [Miners.PoSt] 中的 ChallengeConfidence  配置项
- venus-worker：
  - 适配 venus-market 对于 oss piece store 的支持
  - 支持指定阶段批次启动 [#144](https://github.com/ipfs-force-community/venus-cluster/issues/144)
  - 支持外部处理器根据权重分配任务 [#145](https://github.com/ipfs-force-community/venus-cluster/issues/145)
  - 支持新的订单填充逻辑：
    - 禁止cc扇区 [#161](https://github.com/ipfs-force-community/venus-cluster/issues/161)
    - `min_used_space` [#183](https://github.com/ipfs-force-community/venus-cluster/pull/183)
  - 日志输出当前时区时间 [#87](https://github.com/ipfs-force-community/venus-cluster/issues/87)
  - 配置调整：
    - 废弃 [processors.limit] 配置块，替换为 [processors.limitation.concurrent] 配置块
    - 新增 [processors.limitation.staggered] 配置块
    - 新增 [[processors.{stage name}]] 中的 weight 配置项
    - 新增 [sealing] 中的 min_deal_space 配置项
    - 新增 [sealing] 中的 disable_cc 配置项
- 工具链：
  - 支持 cuda 版本编译
- 文档：
  - 更多 QA 问答
  - [10.venus-worker任务管理](./docs/zh/10.venus-worker任务管理.md)
  - [11.任务状态流转.md](./docs/zh/11.任务状态流转.md)
- 其他改善和修复



## v0.2.0
- 支持 snapup 批量生产模式
  - venus-worker 支持配置 `snapup` 类型任务
  - venus-sector-manager 支持配置 `snapup` 类型任务
  - venus-sector-manager 新增 `snapup` 相关的命令行工具：
    - `util sealer snap fetch` 用于按 `deadline` 将可用于升级的候选扇区添加到本地
	- `util sealer snap candidates` 用于按 `deadline` 展示可用于升级的本地候选扇区数量
  - 参考文档：[08.snapdeal的支持](https://github.com/ipfs-force-community/venus-cluster/blob/9be393761645f5fbd3a415b5ff1f50ec9254943c/docs/zh/08.snapdeal%E7%9A%84%E6%94%AF%E6%8C%81.md)

- 增强 venus-sector-manager 管理 venus-worker 实例的能力：
  - 新增 venus-worker 定期向 venus-sector-manager 上报一些统计数据的机制
  - 新增 venus-sector-manager 的 `util worker` 工具集

- 增强 venus-sector-manager 根据功能拆分实例的能力：
  - 新增数据代理模式
  - venus-sector-manager 的 `util daemon run` 新增 `--conf-dir` 参数，可以指定配置目录
  - 新增外部证明计算器 (external prover) 的支持
  - 参考文档：[09.独立运行的poster节点](https://github.com/ipfs-force-community/venus-cluster/blob/9be393761645f5fbd3a415b5ff1f50ec9254943c/docs/zh/09.%E7%8B%AC%E7%AB%8B%E8%BF%90%E8%A1%8C%E7%9A%84poster%E8%8A%82%E7%82%B9.md)

- 修复 PreCommit/Prove 的 Batch Commit 未使用相应的费用配置的问题

- 其他调整
  - venus-worker 的配置调整
    - 新增 [[sealing_thread.plan]] 项
	- 新增 [attached_selection] 块，提供 `enable_space_weighted` 项，用于启用以剩余空间为权重选择持久化存储的策略，默认不启用
  - venus-sector-manager 的配置调整
    - 废弃原 [Miners.Deal] 块，调整为 [Miners.Sector.EnableDeals] 项
    - 新增扇区生命周期项 [Miners.Sector.LifetimeDays]
    - 新增 [Miners.SnapUp] 块
	- 新增 [Miners.Sector.Verbose] 项，用于控制封装模块中的部分日志详尽程度
  - venus-sector-manager 的 `util storage attach` 现在默认同时检查 `sealed_file` 与 `cache_dir` 中目标文件的存在性
  - 其他改善和修复

## v0.1.2
- 一些为 SnapUp 支持提供准备的设计和实现变更
- `util storage attach` 新增 `--allow-splitted` 参数， 支持 `sealed_file` 与 `cache_dir` 不处于同一个持久化存储实例中的场景
  参考文档 [06.导入已存在的扇区数据.md#sealed_file-与-cache_dir-分离](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/06.%E5%AF%BC%E5%85%A5%E5%B7%B2%E5%AD%98%E5%9C%A8%E7%9A%84%E6%89%87%E5%8C%BA%E6%95%B0%E6%8D%AE.md#sealed_file-%E4%B8%8E-cache_dir-%E5%88%86%E7%A6%BB)
- 开始整理 `Q&A` 文档
- 添加针对本项目内的组件的统一版本升级工具

## v0.1.1
- 外部处理器支持更灵活的工作模式，包含 **多任务并发**、**自定义锁**，使用方式参考：
  - [`processors.{stage_name} concurrent`](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#processorsstage_name)
  - [`processors.ext_locks`](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/03.venus-worker%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#processorsext_locks)
  - [07.venus-worker外部执行器的配置范例](https://github.com/ipfs-force-community/venus-cluster/blob/main/docs/zh/07.venus-worker%E5%A4%96%E9%83%A8%E6%89%A7%E8%A1%8C%E5%99%A8%E7%9A%84%E9%85%8D%E7%BD%AE%E8%8C%83%E4%BE%8B.md)

- 调整封装过程中获取 `ticket` 的时机，避免出现 `ticket` 过期的情况
- 简单的根据剩余空间量选择持久化存储的机制
- 跟进 `venus-market` 的更新
- 调整和统一日志输出格式
- 其他改善和修复
