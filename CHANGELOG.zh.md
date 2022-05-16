# Changelog

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
