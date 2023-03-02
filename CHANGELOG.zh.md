# Changelog

## v0.4.1
- venus-sector-manager
  - 适配和支持 nv18
  - 修复订单释放 bug [#602](https://github.com/ipfs-force-community/venus-cluster/issues/602)

## v0.4.1-rc2
- venus-sector-manager
  - 升级 jsonrpc 库

## v0.4.1-rc1
- venus-sector-manager
  - 适配和支持 nv18-rc1 [#610](https://github.com/ipfs-force-community/venus-cluster/pull/610)
  - cli 移除 `--net` flag, 自动获取网络参数 [#574](https://github.com/ipfs-force-community/venus-cluster/pull/574)

## v0.4.0
- venus-sector-manager
  - 修改 snapup 对于消息处理和可重试错误的逻辑，在遇到 outofgas 或者钱不足等问题时自行重试 [#545](https://github.com/ipfs-force-community/venus-cluster/pull/545)
  - 修改 cli list sector 中对于不存在 laststate 的结构的扇区不输出其他信息的 bug [#551](https://github.com/ipfs-force-community/venus-cluster/pull/551)

- venus-worker
  - 修复 WindowPoS 无法识别部分错误扇区的 bug [#535](https://github.com/ipfs-force-community/venus-cluster/issues/535)

## v0.4.0-rc5
- venus-sector-manager
  - 适配和支持 nv17 [#521](https://github.com/ipfs-force-community/venus-cluster/pull/521)
  - 修复 Terminate Sector 消息 failed 不能正确退出监听的问题 [#507](https://github.com/ipfs-force-community/venus-cluster/issues/507)
  - 修复在开启消息聚合消息，同时关闭从ctrl地址发送质押，生成消息时，没有包含扇区的问题 [#510](https://github.com/ipfs-force-community/venus-cluster/issues/510)
  - 修复 SnapUp 没有正确处理 errormsg 的问题 [#524](https://github.com/ipfs-force-community/venus-cluster/issues/524)

## v0.4.0-rc4
- venus-sector-manager
  - 适配和支持 nv17-rc5

- venus-worker
  - 修复 venus-worker 配置 `enable_deals=false` 时触发的 bug [#501](https://github.com/ipfs-force-community/venus-cluster/issues/501)

## v0.4.0-rc3-nv17
- venus-sector-manager
  - 适配和支持 nv17-rc3
  - 兼容 venus v1.8.0-rc3 引入了配置项 `GasOverPremium`, `feecap` 和 `maxfee`. 具体参考[配置文档](./docs/zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md) [#337]

## v0.4.0-rc3
- venus-sector-manager
  - 跟踪消息上链逻辑增加状态为 ReplaceMsg 的消息处理 [#435](https://github.com/ipfs-force-community/venus-cluster/issues/435)
- venus-worker
  - 新增 add_pieces 外部执行器。 目的是为了进行并发控制, 避免所有的 add_pieces 同时启动，导致内存不足。 [#403](https://github.com/ipfs-force-community/venus-cluster/issues/403)
  - vc-processors 优化日志输出。[#348](https://github.com/ipfs-force-community/venus-cluster/issues/348)
  - 移除过大尺寸的日志。[#417](https://github.com/ipfs-force-community/venus-cluster/pull/417)

## v0.4.0-rc2
- venus-sector-manager
  - 引入 venus v1.6.1; 引入 filecoin-ffi 内存泄漏修复后的版本; 引入 lotus v1.17.0 并进行兼容性修复 [#331](https://github.com/ipfs-force-community/venus-cluster/issues/331)

- venus-worker
  - 修复venus-worker-util hwinfo 报错 segmentation fault. [#341](https://github.com/ipfs-force-community/venus-cluster/issues/341)
  - 移除预分配 PC1 NUMA aware 内存的功能,避免潜在风险。venus-worker v0.5 规划了更好的 PC1 HugeTLB files 功能 [#350](https://github.com/ipfs-force-community/venus-cluster/issues/350).

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
    - 增加 `[[Common.PersistStores]]` 中的 `AllowMiners ` 和 `DenyMiners` 配置项
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
  - 支持可配置的 `WindowPoSt Chanllenge Confidential` [#163](https://github.com/ipfs-force-community/venus-cluster/issues/163)
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
  - 新增 venus-worker 定期向 venus-secotr-manager 上报一些统计数据的机制
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
	- 新增 [attched_selection] 块，提供 `enable_space_weighted` 项，用于启用以剩余空间为权重选择持久化存储的策略，默认不启用
  - venus-secotr-manager 的配置调整
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
