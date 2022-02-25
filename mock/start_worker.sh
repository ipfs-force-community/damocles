#!/bin/sh

set -e

./dist/bin/venus-worker daemon -c ./venus-worker/assets/venus-worker.mock.toml
