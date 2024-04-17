BUILD_GIT_FALLBACK := "Unknown (no git or not git repo)"
export GIT_COMMIT?=git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2> /dev/null || git rev-parse --short HEAD 2> /dev/null || echo ${BUILD_GIT_FALLBACK}))
export RUSTFLAGS
export RUST_TOOLCHAIN?=$(shell awk 'NR==2 { gsub(/"/, "", $$3); print $$3 }' ./rust-toolchain.toml)

unexport FFI_USE_CUDA
unexport VENUS_WORKER_FEATURES
unexport DAMOCLES_WORKER_FEATURES

ifdef FFI_USE_CUDA
	VENUS_WORKER_FEATURES+=cuda
	DAMOCLES_WORKER_FEATURES+=cuda
endif

ifneq ($(strip $(DAMOCLES_WORKER_FEATURES)),)
	WORKER_FEATURE_FLAGS+=--features="$(strip $(DAMOCLES_WORKER_FEATURES))"
else ifneq ($(strip $(VENUS_WORKER_FEATURES)),)
	WORKER_FEATURE_FLAGS+=--features="$(strip $(VENUS_WORKER_FEATURES))"
endif


all: fmt clippy build-all

fix: fmt clippy
check: check-fmt check-clippy

build-all:
	cargo build --release --workspace $(WORKER_FEATURE_FLAGS)

test-all:
	cargo test --release --workspace -- --nocapture

fmt:
	cargo fmt --all

check-fmt:
	cargo fmt --all -- --check

clippy:
	cargo clippy --release --fix --no-deps --workspace --tests

check-clippy:
	cargo clippy --release --no-deps --workspace --tests -- -D warnings
	# avoid missing checks like #![deny(missing_docs)]
	cargo clippy --release --no-deps --workspace -- -D warnings

dev-env:
	rustup component add --toolchain ${RUST_TOOLCHAIN} rls rust-analysis rust-src rustc-dev clippy rustfmt

publish-vc-processors:
	cargo publish -p vc-processors --features="builtin ext-producer"

upgrade-filecoin-proofs:
	cargo update -p filecoin-proofs
	cargo update -p storage-proofs-core
	cargo update -p storage-proofs-porep
	cargo update -p storage-proofs-post
	cargo update -p storage-proofs-update
	cargo update -p filecoin-proofs-api

TAG?=$(shell git describe --tag 2> /dev/null || git rev-parse --short HEAD 2> /dev/null)
docker:
	docker build \
		-t damocles-worker \
		--build-arg HTTPS_PROXY=${BUILD_DOCKER_PROXY} \
		--build-arg BUILD_TARGET=damocles-worker \
		--build-arg RUST_TOOLCHAIN=${RUST_TOOLCHAIN} \
		--build-arg GIT_COMMIT=${GIT_COMMIT} \
		.

