### **Hardware resource**

```tex
cpu: 32 core
RAM: 1.5 TiB
2*nvme: 7.5 TiB
GPU: 1
```



### **Operation mode**   

```tex
[local] pc1=16  (16*64=1T)
[local] pc2=1   64G
[local] c2=1    192G

RAM:12*64+192+4200=1.25 T
```



### **venus-worker.toml**

```toml
[sealing]
allowed_miners=[34600]
allowed_sizes=["32GiB"]
#enable_deals=false
max_retries=3
#seal_interval="30s"
#recover_interval="30s"
#rpc_polling_interval="30s"
#ignore_proof_check=false

#[nvme01]
[[sealing_thread]]
location="/sealdata1/test1"

[[sealing_thread]]
location="/sealdata1/test2"

[[sealing_thread]]
location="/sealdata1/test3"

[[sealing_thread]]
location="/sealdata1/test4"

[[sealing_thread]]
location="/sealdata1/test5"

[[sealing_thread]]
location="/sealdata1/test6"

[[sealing_thread]]
location="/sealdata1/test7"

[[sealing_thread]]
location="/sealdata1/test8"

#[nvme02]
[[sealing_thread]]
location="/sealdata2/test1"

[[sealing_thread]]
location="/sealdata2/test2"

[[sealing_thread]]
location="/sealdata2/test3"

[[sealing_thread]]
location="/sealdata2/test4"

[[sealing_thread]]
location="/sealdata2/test5"

[[sealing_thread]]
location="/sealdata2/test6"

[[sealing_thread]]
location="/sealdata2/test7"

[[sealing_thread]]
location="/sealdata2/test8"

[[attached]]
name="Tyler"
location="/calib-store"

[processors.limit]
pc1=5
pc2=1
c2=1

[processors.static_tree_d]
32GiB="/root/file-coins/filecoin-proof-parameters/tree_d_all_zero_34359738368"

#fieldsforpc1processors
[[processors.pc1]]
numa_preferred=0
cgroup.cpuset="0-19"
concurrent=5
envs={FIL_PROOFS_MAXIMIZE_CACHING="1",FIL_PROOFS_USE_MULTICORE_SDR="1"}

#P1、C2 vie for lock file
[processors.ext_locks]
gpu=1

[[processors.pc2]]
locks=["gpu"]
cgroup.cpuset="20-31"
concurrent=1
envs={FIL_PROOFS_MAX_GPU_COLUMN_BATCH_SIZE="8000000",FIL_PROOFS_MAX_GPU_TREE_BATCH_SIZE="8000000",FIL_PROOFS_USE_GPU_COLUMN_BUILDER="1",FIL_PROOFS_USE_GPU_TREE_BUILDER="1",CUDA_VISIBLE_DEVICES="0"}

[[processors.c2]]
locks=["gpu"]
cgroup.cpuset="20-31"
concurrent=1
envs={BELLMAN_CUSTOM_GPU="NVIDIAGeForceRTX3080:8704",CUDA_VISIBLE_DEVICES="0"}
```



 **Due to the problem of sharing GPUs in this scheme, some C2 tasks will be overstocked**

By **@Typle**
