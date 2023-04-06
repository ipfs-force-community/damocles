#!/bin/sh

set -e

./dist/damocles-worker daemon -c ./damocles-worker/assets/damocles-worker.mock.toml
