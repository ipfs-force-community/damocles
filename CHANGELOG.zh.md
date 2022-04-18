# Changelog

## v0.2.0rc-1
- 支持 snapup 批量生产模式
  - venus-worker 支持配置 `snapup` 类型任务
  - venus-sector-manager 支持配置 `snapup` 类型任务
  - venus-sector-manager 新增 `snapup` 相关的命令行工具：
    - `util sealer snap fetch` 用于按 `deadline` 将可用于升级的候选扇区添加到本地
	- `util sealer snap candidates` 用于按 `deadline` 展示可用于升级的本地候选扇区数量
- venus-worker 的配置调整
  - 新增 [[sealing_thread.plan]] 项
- venus-secotr-manager 的配置调整
  - 废弃原 [Miners.Deal] 块，调整为 [Miners.Sector.EnableDeals] 项
  - 新增扇区生命周期项 [Miners.Sector.LifetimeDays]
  - 新增 [Miners.SnapUp] 块

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
