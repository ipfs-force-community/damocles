#!/bin/sh
set -e

if [[ -z "$1" ]]
then
	echo "no version provided"
	exit 0
fi

echo "check git changes"
./scripts/check-git-dirty.sh

sed -i "3c version = \"$1\"" ./venus-worker/Cargo.toml
echo "venus-worker version upgraded"

sed -i "s/const Version = .*$/const Version = \"$1\"/g" ./venus-sector-manager/ver/ver.go
# sed -i "3c const Version = \"$1\"" ./venus-sector-manager/ver/ver.go
echo "venus-sector-manager version upgraded"

make all

git commit -am "chore(ver): upgrade to $1"
