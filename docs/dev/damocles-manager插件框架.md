# damocles-manager 插件支持
Inspired by [TIDB plugin](https://github.com/pingcap/tidb/blob/master/docs/design/2018-12-10-plugin-framework.md)

## 背景

damocles 希望给用户提供足够灵活的自定义能力。例如[外部执行器](../zh/07.damocles-worker%E5%A4%96%E9%83%A8%E6%89%A7%E8%A1%8C%E5%99%A8%E7%9A%84%E9%85%8D%E7%BD%AE%E8%8C%83%E4%BE%8B.md)提供了 damocles 与其他进程通信的能力，用户可以使用外部执行器功能创建自定义的优化后的封装算法。本文介绍 damocles-manager 中另一种更轻量级，性能更高的插件机制。两者适用于不同的场景，前者主要用于自定义 filecoin 各个封装算法以及 WindowPoSt、WinningPoSt。后者将用于 damocles-manager 内部自定义数据库类型、自定义存储类型、自定义数据同步、审计工作，亦或是自定义的 deals 匹配算法等。

## 使用
damocles-manager 插件机制基于 [Go plugin](https://pkg.go.dev/plugin#section-documentation)。并提供了统一的插件 Manifest 机制、Package 和灵活的 SPI。

插件开发者创建一个插件需要以下 5 步。
1. 选择一个插件类型，或者发送 PR 给 damocles-manager 创建一个新的插件类型。
2. 创建一个普通的 go package，并创建 `manifest.toml` 文件。

	`manifest.toml` 文件的内容会被解析为 `manager-plugin/spi.go` 中的 `{Kind}Manifest` 结构体。

	[manifest.toml example:](https://github.com/ipfs-force-community/damocles/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/manager-plugin/examples/memdb/manifest.toml#L3-L10)
	```toml
	# manifest.toml
	
	# 插件名称
	name = "memdb"
	# 插件描述
	description = "kvstore in memory"
	# 插件类型，当前支持: FileStore | KVStore | SyncSectorState
	kind = "KVStore"
	# 指定插件的初始化函数
	onInit = "OnInit"
	# 指定插件的 Shutdown 函数
	# onShutdown = "OnShutdown"
	
	# 导出此类型插件的特有方法
	export = [
		{extPoint="Constructor", impl="Open"},
	]
   ```
3. 实现 `Init` 和 `Shutdown` 方法，所有的插件都需要实现这两个方法。
4. 编写特定类型插件的特有方法实现插件逻辑
5. 使用 `cmd/plugin` 命令编译插件，并且将编译后的 `.so` 文件移到 damocles-manager 的[插件目录](../zh/04.damocles-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonplugins)中。`go run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- build --src-dir=./ --out-dir=./`

启动 damocles-manager 后会以日志的形式输出所有成功加载的插件。
```
2023-01-13T11:33:09.658+0800    INFO    dep     dep/sealer_constructor.go:132   loaded plugin 'KVStore/memdb', build time: '2023.01.13 11:32:43'.
2023-01-13T11:33:09.658+0800    INFO    dep     dep/sealer_constructor.go:132   loaded plugin 'FileStore/examplefstore', build time: '2023.01.13 11:32:43'.
...
```
---

### 当前支持的插件类型
#### FileStore 插件

`FileStore` 允许用户创建自定的存储类型，受 filecoin 封装算法限制，目前只支持文件系统，但是 FileStore 插件支持用户自定义扇区文件的存储路径。

##### Manifest:

struct 定义：
```go
type FileStoreManifest struct {
	Manifest

	Constructor func(cfg filestore.Config) (filestore.Store, error)
}
```

manifest.toml 示例：
```toml
# manifest.toml

name = "mystore"
description = "my filestore plugin"
# 插件类型设置为: FileStore
kind = "FileStore"
onInit = "OnInit"
onShutdown = "OnShutdown"

export = [
	# `impl` is your function name
	{extPoint="Constructor", impl="Open"},
]
```

`FileStore` 插件只需要额外提供一个 `Constructor` 函数返回实现了 [filestore.Store](https://github.com/ipfs-force-community/damocles/blob/68b14f09025eab2ed7adb5b86ebb0b098fb3063f/manager-plugin/filestore/filestore.go#L75-L103) 接口的对象即可。

`FileStore` 目前用于 Piece 文件存储 (PieceStore) 与封装后的扇区数据存储 (PersistStores)， 当正确配置 FileStore 插件后， damocles-manager 会调用插件返回的 `filesotre.Store` 进行文件读写。

##### PieceStore 插件配置样例

```toml

[Common.Plugins]
Dir = "path/to/damocles-manager-plugins-dir"

# ...

# damocles-manager 可以配置多个 PieceStore，每个 PieceStore 都可以使用不同的插件。
[[Common.PieceStores]]
Name = "my-file-store"
Path = "/path"

# 指定插件名称
# 注意: PluginName 不是插件程序的文件名，而是在 manifest.toml 中配置的名称
PluginName = "mystore"

[[Common.PieceStores.Meta]]
# Your plugin config here
SubDirPattern = "sub-%d-%d"
# ConfigPath = "path/to/my-store-config.toml"
```

PersistStores 的插件配置与 PieceStore 类似，详细请参考 damocles-manager 配置文件说明。
- [PieceStore 配置说明](../zh/04.damocles-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonpiecestores)
- [PersistStores 配置说明](../zh/04.damocles-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonpersiststores)

##### Example: [fsstore](https://github.com/ipfs-force-community/damocles/tree/main/manager-plugin/examples/fsstore)

---

#### KVStore 插件

`KVStore` 插件允许用户使用其他 damocles-manager 不支持的数据库作为 damocles-manager 的扇区元数据信息存储。

##### Manifest:
struct 定义：
```go
type KVStoreManifest struct {
	Manifest

	Constructor func(meta map[string]string) (kvstore.DB, error)
}
```


manifest.toml 示例：
```toml
# manifest.toml

name = "kvstoreredis"
description = "use redis as kvstore"
# 插件类型设置为：KVStore
kind = "KVStore"
onInit = "OnInit"
onShutdown = "OnShutdown"

export = [
	# `impl` is your function name
	{extPoint="Constructor", impl="Open"},
]
```


`KVStore` 插件也只需要额外提供一个 `Constructor` 函数返回实现了 [kvstore.DB](https://github.com/ipfs-force-community/damocles/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/damocles-manager/pkg/kvstore/kv.go#L39-L46) 接口的对象即可。

##### KvStore 插件配置样例

```toml
[Common.Plugins]
Dir = "path/to/damocles-manager-plugins-dir"

# ...

#### 基础配置范例：
[Common.DB]
# 指定使用 kvstore 插件实现的数据库
Driver = "plugin"

[Common.DB.Plugin]
# 指定插件名称
# 注意：PluginName 不是插件程序的文件名，而是在 manifest.toml 中配置的名称
PluginName = "redis"

# Meta 数据会传入 kvstore 插件的 Constructor 函数中
[Common.DB.Plugin.Meta]
Addr = "127.0.0.1:6379"
# RedisConfigPath = "path/to/my-s3-store-config.toml"
```


##### Example: [memdb](https://github.com/ipfs-force-community/damocles/tree/main/manager-plugin/examples/memdb)

---

#### RegisterJsonRpcManifest 插件
`RegisterJsonRpcManifest` 插件允许用户注册自定义的 jsonrpc 接口到 damocles-manager 中。

Manifest:

Struct 定义：
```go
type RegisterJsonRpcManifest struct {
	Manifest
	
	// Handler returns the jsonrpc namespace and handler
	// See: https://github.com/ipfs-force-community/go-jsonrpc/blob/4e8fb6324df7a31eaa6b480ef9e2a175545ba04b/server.go#L137
	Handler func() (namespace string, handler interface{})
}
```
manifest.toml 示例：
```toml
# manifest.toml

name = "AtomicCounter"
description = "Expose a series of jsonrpc methods to implement an atomic counter"
# 插件类型设置为：RegisterJsonRpc
kind = "RegisterJsonRpc"
onInit = "OnInit"
onShutdown = "OnShutdown"

export = [
	{extPoint="Handler", impl="Handler"},
]
```

`RegisterJsonRpc` 插件无需配置，当 damocles-manager 在插件目录中扫描到多个 `RegisterJsonRpc` 插件时，会[依次调用](https://github.com/ipfs-force-community/damocles/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/damocles-manager/modules/impl/sectors/state_mgr.go#L181)每一个 `RegisterJsonRpc` 插件。我们可以通过编写多个的 `RegisterJsonRpc` 插件注册多个自定义 jsonrpc 接口。

---

#### SyncSectorState 插件 (未稳定的插件类型)
`SyncSectorState` 插件允许用户编写插件同步 damocles-manager 扇区变动信息。因为 KvStore 存储的是二进制数据，无法获取扇区结构化数据，本插件提供结构化变更数据，且允许多个 SyncSectorState 插件同时工作。

Manifest:

Struct 定义：
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
manifest.toml 示例：
```toml
# manifest.toml

name = "mysqlsyncer"
description = "sync sectors state to mysql"
# 插件类型设置为：SyncSectorState
kind = "SyncSectorState"
onInit = "OnInit"
onShutdown = "OnShutdown"

export = [
	{extPoint="OnImport", impl="OnImport"},
	{extPoint="OnInit", impl="OnInit"},
	{extPoint="OnUpdate", impl="OnUpdate"},
	{extPoint="OnFinalize", impl="OnFinalize"},
	{extPoint="OnRestore", impl="OnRestore"},
]
```

`SyncSectorState` 插件无需配置，当 damocles-manager 在插件目录中扫描到多个 `SyncSectorState` 插件时，会[依次调用](https://github.com/ipfs-force-community/damocles/blob/dfc20a9a4d2728192bbbf830ddfd15b684b98ce9/damocles-manager/modules/impl/sectors/state_mgr.go#L181)每一个 `SyncSectorState` 插件。我们可以通过编写不同的 `SyncSectorState` 插件将扇区状态数据同步到不同的目标数据库或者存储中。

---

#### 其他插件
正如前文所说的：
> 后者将用于 damocles-manager 内部自定义数据库类型、自定义存储类型、自定义数据同步、审计工作，亦或是自定义的 deals 匹配算法等。

damocles-manager 未来会提供更多类型的插件。也欢迎给 damocles-manager 发送 issue 和 pr，交流想法与代码。


### 限制
damocles-manager 当前存在以下限制：

- 插件仅能使用 go 编写
- damocles-manager 与 plugin 的共同依赖包的版本必须一致

针对「damocles-manager 与 plugin 的共同依赖包的版本必须一致」的问题，damocles-manager 提供了依赖版本检查工具可以自动修复或检查插件依赖版本。

```shell
# 仅检查依赖版本
go run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- check-dep /path/to/go.mod

# 自动修复依赖版本
go run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- check-dep --fix /path/to/go.mod
```
