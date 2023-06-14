# Metrics（指标监控）的实践



## common

在 `damocles` 的开发维护过程中，对于 `metrics` 即指标监控的处理应当遵循一些基本的原则。



### 类型

通常来说，我们使用以下三类指标来进行采集和记录：

1. 计数器（`Counter`）通常是一类累加、递增的数值，也可以使用绝对值进行设置；
2. 测量值（`Gauge`）通常是一类可能上下浮动的数值，也可以使用绝对值进行设置；
3. 直方图（`Histogram`）是一类需要经过统计、聚合产生结果的数值。



注意：

- 当始终使用绝对值进行设置时，计数器与测量值并无明显差异。
- 直方图类型能够支持的统计、聚合，通常有赖于所使用的框架提供相应计算，这种计算：
  - 通常是隐式完成的，即不需要使用者主动调用
  - 使用者通常可以配置一些简单的策略，如分位数（`quantiles`）等



### 采集

对于我们使用的采集端，通常遵循：

- 使用 `prometheus` 作为采集组件
- 使用 `exporter` 模式接入 `prometheus`



### 实现

对于开发过程中所选用的依赖库/框架，以及自行定制封装的部分，我们通常遵循：

1. 提供必要的数据类型
2. 便于集成
3. 对于 key、label names 这类属性，尽可能避免重复手写，尽可能提供语法提示



## metrics in damocles-worker 

### 选型

`damocles-worker` 中的 `metrics` 部分选用了 [metrics](https://crates.io/crates/metrics) 作为基础框架，并通过 [metrics-exporter-prometheus](https://crates.io/crates/metrics-exporter-prometheus) 接入 `prometheus`.



### 封装

参考 TiKV 所使用的 [prometheus-static-metric](https://crates.io/crates/prometheus-static-metric) 的范式，`damocles-worker` 编写了用于构造指标对象和指标视图（View）的宏。



### 示例

使用者可以通过以下方式构造一个 `View` 类型，和与之对对应的静态对象 `static VIEW: View`

```rust
mod metrics {
    mod rpc {
        make_metric! {
            (call: counter, "rpc.call", method),
            (error: counter, "rpc.error", method),
            (timing: histogram, "rpc.timing", method),
        }
    }
}

```

并通过类似：

```rust
crate::metrics::rpc::VIEW.call.method("example").incr();

let now = std::time::Instant::now();
let res = do_some_rpc_call();

crate::metrics::rpc::VIEW.timing.method("example").record(now.elapsed());

if res.is_err() {
	crate::metrics::rpc::VIEW.error.method("example").incr();
}
```

的方式进行使用





## metrics in damocles-manager

`TODO`

