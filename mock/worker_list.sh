#!/bin/sh

set -e
./dist/damocles-worker worker --config ./damocles-worker/assets/damocles-worker.mock.toml list
