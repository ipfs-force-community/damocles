#!/bin/sh

set -e
./dist/bin/damocles-worker worker --config ./damocles-worker/assets/damocles-worker.mock.toml list
