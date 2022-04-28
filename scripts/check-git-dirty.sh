#!/bin/bash
set -e

git status --porcelain
test -z "$(git status --porcelain)"
