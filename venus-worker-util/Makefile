export GIT_COMMIT=git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))

all: fmt clippy build-all

test-all:
	cargo test --release --workspace -- --nocapture

check-all: check-fmt check-clippy

build-all:
	cargo build --release

fmt:
	cargo fmt --all

check-fmt:
	cargo fmt --all -- --check

clippy:
	cargo clippy --release --fix --no-deps --tests

check-clippy:
	cargo clippy --release --no-deps --tests -- -D warnings
	# avoid missing checks like #![deny(missing_docs)]
	cargo clippy --release --no-deps -- -D warnings

dev-env:
	rustup component add clippy rustfmt

TAG:=test
docker:
	curl -O https://raw.githubusercontent.com/filecoin-project/venus-docs/master/script/docker/dockerfile
	docker build --build-arg https_proxy=$(BUILD_DOCKER_PROXY) --build-arg BUILD_TARGET=venus-worker-util -t venus-worker-util .
	docker tag venus-worker-util filvenus/venus-worker-util:$(TAG)
