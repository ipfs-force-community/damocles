#!/bin/sh
set -e

if [[ -z "$1" ]]
then
	echo "no version provided"
	exit 0
fi

echo "check git changes"
./scripts/check-git-dirty.sh

sed -i "3c version = \"$1\"" ./venus-worker/vc-processors/Cargo.toml
echo "vc-processors version upgraded"

make build-worker

git commit -am "chore(ver): vc-processors upgrade to $1"
