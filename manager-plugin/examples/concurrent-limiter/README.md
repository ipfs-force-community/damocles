# damocles-manager 全局并发限制插件

### 原理
damocles-manager 全局并发限制插件，实际上是往 damocles-manager 添加了两个新的 jsonrpc api 接口:
1. 获取锁: DAMOCLES_LIMITER.Acquire
2. 释放锁: DAMOCLES_LIMITER.Release
每个 damocles-worker 都请求 damocles-manager 的这两个 rpc 接口实现全局并发限制功能。

### 编译 (请使用与 damocles-manager 相同版本的 go 编译器)
```
make build-all
```
编译后会生成 `plugin-concurrent-limiter.so` 插件文件

### 使用
#### 1. 将 `plugin-concurrent-limiter.so` 拷贝到 damocles-manager 的插件目录 [文档](https://github.com/ipfs-force-community/damocles/blob/main/docs/zh/04.damocles-manager%E7%9A%84%E9%85%8D%E7%BD%AE%E8%A7%A3%E6%9E%90.md#commonplugins)
damocles-manager 的插件目录配置在 damocles-manager 的配置文件中。
```toml
#  ~/.damocles-manager/sector-manager.cfg

# ...

[Common.Plugins]
Dir = "/path/to/your-damocles-manager-plugins-dir"

# ...
```

#### 2. 插件配置

配置文件位于 damocles-manager 插件目录且名称固定为 `concurrent-limiter.toml`

***首次启动带本插件的 damocles-manager 时会自动创建配置文件***

```toml
#  /path/to/your-damocles-manager-plugins-dir/concurrent-limiter.toml

# Default config:
# 本地数据库配置
#DataDir = "/path/to/your-damocles-manager-plugins-dir/.concurrent-limiter"
#
[Concurrent]
# add_pieces 并发限制为 2
add_pieces = 2
# ...

```

#### 3. 重启 damocles-manager。

#### 4. 配置 damocles-worker

```toml
# path/to/your-damocles-worker.toml

[processors.limitation.concurrent]
# 单机并发限制
add_pieces = 10

# 默认的执行器不支持通过此插件限制并发，需要稍加改造
[[processors.add_pieces]]
bin="dist/bin/my-add-pieces-processor"
args = ["processor", "add_pieces",  "--manager_addr", "/ip4/127.0.0.1/tcp/1789"]
```
