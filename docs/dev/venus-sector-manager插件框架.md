# venus-sector-manager 插件支持
Inspired by [TIDB plugin](https://github.com/pingcap/tidb/blob/master/docs/design/2018-12-10-plugin-framework.md)

## 背景

venus-cluster 希望给用户提供足够灵活的自定义能力。例如[外部执行器](../zh/07.venus-worker%E5%A4%96%E9%83%A8%E6%89%A7%E8%A1%8C%E5%99%A8%E7%9A%84%E9%85%8D%E7%BD%AE%E8%8C%83%E4%BE%8B.md)提供了 venus-cluster 与其他进程通信的能力，用户可以使用外部执行器功能创建自定义的优化后的封装算法。本文介绍 venus-sector-manager 中另一种更轻量级，性能更高的插件机制。两者适用于不同的场景，前者主要用于自定义 filecoin 各个封装算法以及 WindowPoSt、WinningPoSt。后者将用于 venus-sector-manager 内部自定义数据库类型、自定义存储类型、自定义数据同步、审计工作，亦或是自定义的 deals 匹配算法等。

## 使用
venus-sector-manager 插件机制基于 [Go plugin](https://pkg.go.dev/plugin#section-documentation)。 并提供了统一的插件 Manifest 机制、Package 和灵活的 SPI。

插件开发者创建一个插件需要以下 5 步。
1. 选择一个插件类型，或者发送 PR 给 venus-sector-manager 创建一个新的插件类型。
2. 创建一个普通的 go package，并创建 `manifest.toml` 文件。[example](https://github.com/ipfs-force-community/venus-cluster/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/venus-sector-manager/plugin/examples/memdb/manifest.toml#L3-L10)
3. 实现 `Init` 和 `Shutdown` 方法，所有的插件都需要实现这两个方法。
4. 编写特定类型插件的特有方法实现插件逻辑
5. 使用 `cmd/buildplugin` 命令编译插件，并且将编译后的 `.so` 文件移到 venus-sector-manager 的[插件目录](../zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonplugins)中。`go run github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/buildplugin@latest --src-dir=./ --out-dir=./`

启动 venus-sector-manager 后会以日志的形式输出所有成功加载的插件。
```
2023-01-13T11:33:09.658+0800    INFO    dep     dep/sealer_constructor.go:132   loaded plugin 'KVStore/memdb', build time: '2023.01.13 11:32:43'.
2023-01-13T11:33:09.658+0800    INFO    dep     dep/sealer_constructor.go:132   loaded plugin 'ObjStore/fsstore', build time: '2023.01.13 11:32:43'.
...
```
---

### 当前支持的插件类型
#### ObjStore 插件

`ObjStore` 允许用户创建自定的存储类型，例如 s3, fs 等。

##### Manifest:
```go
type ObjStoreManifest struct {
	Manifest

	Constructor func(cfg objstore.Config) (objstore.Store, error)
}
```

`ObjStore` 插件只需要额外提供一个 `Constructor` 函数返回实现了 [objstore.Store](https://github.com/ipfs-force-community/venus-objstore/blob/00ad77fcbfed1df5c1613176521bce3ba3041fc7/objstore.go#L50-L61) 接口的对象即可。

`ObjStore` 目前用于 Piece 文件存储 (PieceStore) 与封装后的扇区数据存储 (PersistStores)， 当正确配置 ObjStore 插件后， venus-sector-manager 会调用插件返回的 `objstore.Store` 进行文件读写。

##### PieceStore 插件配置样例

```toml

[Common.Plugins]
Dir = "path/to/vsm-plugins-dir"

# ...

# vsm 可以配置多个 PieceStore， 每个 PieceStore 都可以使用不同的插件。
[[Common.PieceStores]]
Name = "my-s3-store"
Path = "/path"

# 指定插件名称
# 注意: PluginName 不是插件程序的文件名，而是在 manifest.toml 中配置的名称
PluginName = "s3store"

[[Common.PieceStores.Meta]]
# Your plugin config here
Bucket = "mybucket"
# ConfigPath = "path/to/my-s3-store-config.toml"
```

PersistStores 的插件配置与 PieceStore 类似，详细请参考 venus-sector-manager 配置文件说明。
- [PieceStore 配置说明](../zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonpiecestores)
- [PersistStores 配置说明](../zh/04.venus-sector-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonpersiststores)

##### Example: [fsstore](https://github.com/ipfs-force-community/venus-cluster/tree/feat/0x5459/pluggable_database_support/venus-sector-manager/plugin/examples/fsstore)

---

#### KVStore 插件

`KVStore` 插件允许用户使用其他 venus-sector-manager 不支持的数据库作为 venus-sector-manager 的扇区元数据信息存储。

##### Manifest:
```go
type KVStoreManifest struct {
	Manifest

	Constructor func(meta map[string]string) (kvstore.DB, error)
}
```

`KVStore` 插件也只需要额外提供一个 `Constructor` 函数返回实现了 [kvstore.DB](https://github.com/ipfs-force-community/venus-cluster/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/venus-sector-manager/pkg/kvstore/kv.go#L39-L46) 接口的对象即可。

##### KvStore 插件配置样例

```toml
[Common.Plugins]
Dir = "path/to/vsm-plugins-dir"

# ...

#### 基础配置范例:
[Common.DB]
# 指定使用 kvstore 插件实现的数据库
Driver = "plugin"

[Common.DB.Plugin]
# 指定插件名称
# 注意: PluginName 不是插件程序的文件名，而是在 manifest.toml 中配置的名称
PluginName = "redis"

# Meta 数据会传入 kvstore 插件的 Constructor 函数中
[Common.DB.Plugin.Meta]
Addr = "127.0.0.1:6379"
# RedisConfigPath = "path/to/my-s3-store-config.toml"
```


##### Example: [memdb](https://github.com/ipfs-force-community/venus-cluster/tree/feat/0x5459/pluggable_database_support/venus-sector-manager/plugin/examples/memdb)

---

#### SyncSectorState 插件 (未稳定的插件类型)
`SyncSectorState` 插件允许用户编写插件同步 venus-sector-manager 扇区变动信息。因为 KvStore 存储的是二进制数据，无法获取扇区结构化数据，本插件提供结构化变更数据，且允许多个 SyncSectorState 插件同时工作。

Manifest:
```go
type SyncSectorStateManifest struct {
	Manifest

	OnImport   func(...) error
	OnInit     func(...) error
	OnUpdate   func(...) error
	OnFinalize func(...) error
	OnRestore  func(...) error
}
```

`SyncSectorState` 插件无需配置，当 venus-sector-manager 在插件目录中扫描到多个 `SyncSectorState` 插件时, 会[依次调用](https://github.com/ipfs-force-community/venus-cluster/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/venus-sector-manager/modules/impl/sectors/state_mgr.go#L181)每一个 `SyncSectorState` 插件。我们可以通过编写不同的 `SyncSectorState` 插件将扇区状态数据同步到不同的目标数据库或者存储中。

---

#### 其他插件
正如前文所说的:
> 后者将用于 venus-sector-manager 内部自定义数据库类型、自定义存储类型、自定义数据同步、审计工作，亦或是自定义的 deals 匹配算法等。

venus-sector-manager 未来会提供更多类型的插件。也欢迎给 venus-sector-manager 发送 issue 和 pr，交流想法与代码。


### 限制
venus-sector-manager 当前存在以下限制：

- 插件仅能使用 go 编写
- venus-sector-manager 与 plugin 的共同依赖包的版本必须一致
