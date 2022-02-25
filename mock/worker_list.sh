#!/bin/sh

set -e
./dist/bin/venus-worker worker --config ./venus-worker/assets/venus-worker.mock.toml list
