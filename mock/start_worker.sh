#!/bin/sh

set -e

./dist/bin/damocles-worker daemon -c ./damocles-worker/assets/damocles-worker.mock.toml
