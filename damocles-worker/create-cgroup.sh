#! /bin/bash
u=$(whoami)
for subsystem in `ls /sys/fs/cgroup/`
do
	echo /sys/fs/cgroup/${subsystem}
	sudo mkdir -p /sys/fs/cgroup/${subsystem}/vc-worker
	sudo chown -R ${u}: /sys/fs/cgroup/${subsystem}/vc-worker
done
