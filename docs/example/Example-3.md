### **Hardware resource**

```tex
CPU: 32 core
RAM: 2 TiB
NVMe: 30 TiB
GPU: 2
```



### **Operation mode**   

```tex
[local] pc1=24  (16*64=1.5T)
[local] pc2=1   64G
[local] c2=2    2*192=384G

RAM:24*64+64+384=1.93 T
```



### **venus-worker.toml**

```toml
[worker]
name="worker-1"
rpc_server.host="0.0.0.0"
rpc_server.port=17891
[sector_manager]
rpc_client.addr="/ip4/192.168.12.11/tcp/1789"
#rpc_client.headers={User-Agent="jsonrpc-core-client"}
#piece_token=""
[sealing]
allowed_miners=[35398]
allowed_sizes=["32GiB"]
#enable_deals=false
max_retries=3
#seal_interval="30s"
#recover_interval="30s"
#rpc_polling_interval="30s"
#ignore_proof_check=false
[[sealing_thread]]
location="/workdir/sealtset/test01"
[[sealing_thread]]
location="/workdir/sealtset/test02"
[[sealing_thread]]
location="/workdir/sealtset/test03"
[[sealing_thread]]
location="/workdir/sealtset/test04"
[[sealing_thread]]
location="/workdir/sealtset/test05"
[[sealing_thread]]
location="/workdir/sealtset/test06"
[[sealing_thread]]
location="/workdir/sealtset/test07"
[[sealing_thread]]
location="/workdir/sealtset/test08"
[[sealing_thread]]
location="/workdir/sealtset/test09"
[[sealing_thread]]
location="/workdir/sealtset/test10"
[[sealing_thread]]
location="/workdir/sealtset/test11"
[[sealing_thread]]
location="/workdir/sealtset/test12"
[[sealing_thread]]
location="/workdir/sealtset/test13"
[[sealing_thread]]
location="/workdir/sealtset/test14"
[[sealing_thread]]
location="/workdir/sealtset/test15"
[[sealing_thread]]
location="/workdir/sealtset/test16"
[[sealing_thread]]
location="/workdir/sealtset/test17"
[[sealing_thread]]
location="/workdir/sealtset/test18"
[[sealing_thread]]
location="/workdir/sealtset/test19"
[[sealing_thread]]
location="/workdir/sealtset/test20"
[[sealing_thread]]
location="/workdir/sealtset/test21"
[[sealing_thread]]
location="/workdir/sealtset/test22"
[[sealing_thread]]
location="/workdir/sealtset/test23"
[[sealing_thread]]
location="/workdir/sealtset/test24"
[[sealing_thread]]
location="/workdir/sealtset/test25"
[[sealing_thread]]
location="/workdir/sealtset/test26"
[[sealing_thread]]
location="/workdir/sealtset/test27"
[[sealing_thread]]
location="/workdir/sealtset/test28"
[[sealing_thread]]
location="/workdir/sealtset/test29"
[[sealing_thread]]
location="/workdir/sealtset/test30"
[[sealing_thread]]
location="/workdir/sealtset/test31"
[[sealing_thread]]
location="/workdir/sealtset/test32"
[[sealing_thread]]
location="/workdir/sealtset/test33"
[[sealing_thread]]
location="/workdir/sealtset/test34"
[[sealing_thread]]
location="/workdir/sealtset/test35"
[[sealing_thread]]
location="/workdir/sealtset/test36"
[[sealing_thread]]
location="/workdir/sealtset/test37"
[[sealing_thread]]
location="/workdir/sealtset/test38"
[[sealing_thread]]
location="/workdir/sealtset/test39"
[[sealing_thread]]
location="/workdir/sealtset/test40"
[[sealing_thread]]
location="/workdir/sealtset/test41"
[[sealing_thread]]
location="/workdir/sealtset/test42"
[[sealing_thread]]
location="/workdir/sealtset/test43"
[[sealing_thread]]
location="/workdir/sealtset/test44"
[[sealing_thread]]
location="/workdir/sealtset/test45"
[[sealing_thread]]
location="/workdir/sealtset/test46"
[[sealing_thread]]
location="/workdir/sealtset/test47"
[[sealing_thread]]
location="/workdir/sealtset/test48"
[[sealing_thread]]
location="/workdir/sealtset/test49"
[[sealing_thread]]
location="/workdir/sealtset/test50"
[[sealing_thread]]
location="/workdir/sealtset/test51"
[[sealing_thread]]
location="/workdir/sealtset/test52"
[[sealing_thread]]
location="/workdir/sealtset/test53"
[[sealing_thread]]
location="/workdir/sealtset/test54"


[[attached]]
name="store98"
location="/data/write/test"

[processors.limit]
pc1=24
pc2=2
c2=2

[processors.static_tree_d]
32GiB="/workdir/tmple/sc-02-data-tree-d.dat"

[processors.ext_locks]
gpu1=1
gpu2=1

[[processors.pc1]]
numa_preferred=0
cgroup.cpuset="0-35"
concurrent=14
envs={FIL_PROOFS_USE_MULTICORE_SDR="1",FIL_PROOFS_MULTICORE_SDR_PRODUCERS="1"}
[[processors.pc1]]
numa_preferred=1
cgroup.cpuset="48-83"
concurrent=13
envs={FIL_PROOFS_USE_MULTICORE_SDR="1",FIL_PROOFS_MULTICORE_SDR_PRODUCERS="1"}

#Shared gpu-1
[[processors.pc2]]
locks=["gpu1"]
cgroup.cpuset="36-47,84-95"
concurrent=1
envs={FIL_PROOFS_MAX_GPU_COLUMN_BATCH_SIZE="8000000",FIL_PROOFS_MAX_GPU_TREE_BATCH_SIZE="8000000",FIL_PROOFS_USE_GPU_COLUMN_BUILDER="1",FIL_PROOFS_USE_GPU_TREE_BUILDER="1",CUDA_VISIBLE_DEVICES="0"}
[[processors.c2]]
locks=["gpu1"]
cgroup.cpuset="36-47,84-95"
concurrent=1
envs={BELLMAN_CUSTOM_GPU="GeForceRTX3090:10496",CUDA_VISIBLE_DEVICES="0"}

#Shared gpu-2
[[processors.pc2]]
locks=["gpu2"]
cgroup.cpuset="36-47,84-95"
concurrent=1
envs={FIL_PROOFS_MAX_GPU_COLUMN_BATCH_SIZE="8000000",FIL_PROOFS_MAX_GPU_TREE_BATCH_SIZE="8000000",FIL_PROOFS_USE_GPU_COLUMN_BUILDER="1",FIL_PROOFS_USE_GPU_TREE_BUILDER="1",CUDA_VISIBLE_DEVICES="1"}
[[processors.c2]]
locks=["gpu2"]
cgroup.cpuset="36-47,84-95"
concurrent=1
envs={BELLMAN_CUSTOM_GPU="GeForceRTX3090:10496",CUDA_VISIBLE_DEVICES="1"}
```



 **Share the GPU. Pay attention to the GPU lock and display memory utilization, which can be used for reference**

By **@hanxianzhai**
