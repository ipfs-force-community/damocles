#!/bin/sh

set -e

rm -rf ./mock-tmp/store1 ./mock-tmp/store2 ./mock-tmp/store3
rm -rf mock-tmp/remote
./dist/bin/damocles-worker store sealing-init -l ./mock-tmp/store1 ./mock-tmp/store2 ./mock-tmp/store3
./dist/bin/damocles-worker store file-init -l ./mock-tmp/remote
