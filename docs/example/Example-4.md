### **Hardware resource**

```tex
CPU: 16 core
RAM: 512 GiB + swap 96G(on nvme)
NVMe: 7.5 TiB
GPU: 1
```



### **Operation mode**   

```tex
[local] pc1=5  (5*64=320G)
[local] pc2=1   64G
[local] c2=1    192G

RAM:5*64+64+192=576 G
```



### **damocles-worker.toml**

```toml
[worker]
name="ovy"
rpc_server.host="0.0.0.0"
rpc_server.port=17891

[sector_manager]
rpc_client.addr="/ip4/127.0.0.1/tcp/1789"
#rpc_client.headers={User-Agent="jsonrpc-core-client"}
#piece_token="****"

[sealing]
allowed_miners=[35342]
allowed_sizes=["32GiB"]
#enable_deals=false
max_retries=3
#seal_interval="30s"
#recover_interval="30s"
#rpc_polling_interval="30s"
#ignore_proof_check=false

[[sealing_thread]]
location="/mnt/md0/test1"

[[sealing_thread]]
location="/mnt/md0/test2"

[[sealing_thread]]
location="/mnt/md0/test3"

[[sealing_thread]]
location="/mnt/md0/test4"

[[sealing_thread]]
location="/mnt/md0/test5"

[[sealing_thread]]
location="/mnt/md0/test6"

[[sealing_thread]]
location="/mnt/md0/test7"

[[sealing_thread]]
location="/mnt/md0/test8"

[[sealing_thread]]
location="/mnt/md0/test9"

[[sealing_thread]]
location="/mnt/md0/test10"


[[attached]]
name="store10"
location="/mnt/md0/store1"

[processors.limit]
pc1=5
pc2=1
c2=1

[processors.static_tree_d]
32GiB="/var/tmp/filecoin-proof-parameters/sc-02-data-tree-d.dat"

#fieldsforpc1processors
[[processors.pc1]]
#numa_preferred=0
cgroup.cpuset="0-9"
concurrent=5
envs={FIL_PROOFS_USE_MULTICORE_SDR="1",FIL_PROOFS_MULTICORE_SDR_PRODUCERS="1"}

[processors.ext_locks]
gpu=1

[[processors.pc2]]
locks=["gpu"]
cgroup.cpuset="1,3,5,7,9,10-15"
concurrent=1
envs={FIL_PROOFS_MAX_GPU_COLUMN_BATCH_SIZE="8000000",FIL_PROOFS_MAX_GPU_TREE_BATCH_SIZE="8000000",FIL_PROOFS_USE_GPU_COLUMN_BUILDER="1",FIL_PROOFS_USE_GPU_TREE_BUILDER="1",CUDA_VISIBLE_DEVICES="0"}

[[processors.c2]]
locks=["gpu"]
cgroup.cpuset="1,3,5,7,9,10-15"
concurrent=1
envs={BELLMAN_CUSTOM_GPU="GeForceRTX2080Ti:4352",CUDA_VISIBLE_DEVICES="0"}
```



 **Applicable to low configuration machine applications, which may cause some tasks to be slow**

By **@ovy**
