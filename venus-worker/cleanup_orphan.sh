prefix="/sys/fs/cgroup/cpuset/"
for orphan in `ls -d ${prefix}vc-worker/sub-*/`
do
	group=${orphan/#$prefix}
	echo "rm orphan group ${group}"
	cgdelete cpuset:${group}
done
