**Hardware resource**

```tex
cpu: 48 core
RAM: 1 TiB
nvme: 20 TiB
GPU: 1
```



**Operation mode** 

```tex
[local] pc1=12  (12*64=768G)
[local] pc2=1   192G
[remote] c2=2

RAM:12*64+192=968G
```



**venus-worker.toml**

```toml
[worker]
#name="worker-#1"
#rpc_server.host="192.168.1.100"
#rpc_server.port=17891

[sector_manager]
rpc_client.addr="/ip4/192.168.28.32/tcp/1789"
#rpc_client.headers={User-Agent="jsonrpc-core-client"}
#piece_token="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiMS0xMjUiLCJwZXJtIjoic2lnbiIsImV4dCI6IiJ9.JenwgK0JZcxFDin3cyhBUN41VXNvYw-_0UUT2ZOohM0"

[sealing]
allowed_miners=[35201]
allowed_sizes=["32GiB"]
enable_deals=true
#max_deals=3
max_retries=3
#seal_interval="30s"
#recover_interval="30s"
#rpc_polling_interval="30s"
#ignore_proof_check=false

[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store01"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
#sealing.enable_deals=true
#sealing.max_deals=3
#sealing.max_retries=3
#sealing.seal_interval="30s"
#sealing.recover_interval="30s"
#sealing.rpc_polling_interval="30s"
#sealing.ignore_proof_check=false
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store02"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store03"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store04"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store05"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store06"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store07"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store08"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store09"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store10"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store11"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store12"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store13"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store14"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store15"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store16"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store17"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store18"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store19"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store20"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store21"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store22"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store23"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]
[[sealing_thread]]
location="/data/nvme-cache/venus-cluster-worker-cache/cache-store24"
sealing.allowed_miners=[35201]
sealing.allowed_sizes=["32GiB"]

# store path <The name should be the same as that configured in venus-sector-manager>
[[attached]]
name="calib-t035201-sectors"
location="/data/calib-t035201-sectors"

[processors.limit]
pc1=12
pc2=1
c2=2

[processors.ext_locks]
gpu1=1

# tree-d path
[processors.static_tree_d]
#2KiB="./tmp/2k/sc-02-data-tree-d.dat"

#fieldsfortree_dprocessor
[[processors.tree_d]]
#fieldsforpc1processors

[[processors.pc1]]
#bin="./dist/bin/venus-worker-plugin-pc1"
#args=["--args-1","1",--"args-2","2"]
numa_preferred=0
cgroup.cpuset="0-2"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=1
cgroup.cpuset="3-5"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=2
cgroup.cpuset="6-8"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=3
cgroup.cpuset="9-11"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=4
cgroup.cpuset="12-14"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=5
cgroup.cpuset="15-17"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=6
cgroup.cpuset="18-20"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=7
cgroup.cpuset="21-23"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=0
cgroup.cpuset="24-26"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=1
cgroup.cpuset="27-29"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=2
cgroup.cpuset="30-32"
concurrent=1
envs={RUST_LOG="Info"}
[[processors.pc1]]
numa_preferred=3
cgroup.cpuset="33-35"
concurrent=1
envs={RUST_LOG="Info"}

#fieldsforpc2processors
[[processors.pc2]]
cgroup.cpuset="36-47"
locks=["gpu1"]
envs={RUST_LOG="Info"}

#GPU outsourcing to other hosts [https://github.com/ipfs-force-community/gpuproxy]
#fieldsforc2processor
[[processors.c2]]
bin="/home/c2cdeployer/venus/cluster_c2_plugin"
args=["run","--gpuproxy-url","http://192.168.28.32:7788","--log-level","info","--poll-task-interval","60"]

[[processors.c2]]
bin="/home/c2cdeployer/venus/cluster_c2_plugin"
args=["run","--gpuproxy-url","http://192.168.28.36:7788","--log-level","info","--poll-task-interval","60"]

[[processors.c2]]
cgroup.cpuset="36-47"
locks=["gpu1"]
envs={RUST_LOG="Info"}
```

3090GPU：
```bash
gpuproxy/target/release/gpuproxy \
--url 0.0.0.0:7788 \
--log-level info  \
run--db-dsn mysql://venus-gpuproxy:xxxxxx@192.168.28.32:3306/venus_gpuproxy\
--allow-type 0\
--max-tasks 1\
--resource-type fs\
--fs-resource-path /data/gpuproxy-store

```

3090GPU：
```bash
gpuproxy/target/release/gpuproxy \
--url 0.0.0.0:7788 \
--log-level info \
run--db-dsn 'mysql://venus-gpuproxy:xxxxxx@192.168.28.32:3306/venus_gpuproxy_x1803'\
--allow-type 0\
--max-tasks 1\
--resource-type fs\
--fs-resource-path /data/gpuproxy-store
```
**This scheme is currently the best scheme with strong scalability. The disadvantage is that the configuration is complex. All C2 machines need to use the same shared directory**

By **@Dennis Zou**