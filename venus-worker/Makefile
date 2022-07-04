export GIT_COMMIT=git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
export RUSTFLAGS

unexport FFI_USE_CUDA
unexport VENUS_WORKER_FEATURES

ifdef FFI_USE_CUDA
	VENUS_WORKER_FEATURES+=cuda
endif

ifneq ($(strip $(VENUS_WORKER_FEATURES)),)
	VENUS_WORKER_FEATURE_FLAGS+=--features="$(strip $(VENUS_WORKER_FEATURES))"
endif

all: fmt clippy build-all

check-all: check-fmt check-clippy

# Add `--cfg unsound_local_offset` flag to allow time crate to get the local time zone. \
See: https://docs.rs/time/0.3.9/time/index.html#feature-flags and \
https://github.com/time-rs/time/issues/293#issuecomment-1005002386
ifeq ($(findstring --cfg unsound_local_offset, $(RUSTFLAGS)), )
RUSTFLAGS+=--cfg unsound_local_offset
endif

build-all:
	cargo build --release $(VENUS_WORKER_FEATURE_FLAGS)

fmt:
	cargo fmt --all

check-fmt:
	cargo fmt --all -- --check

clippy:
	cargo clippy --fix --no-deps

check-clippy:
	cargo clippy --no-deps -- -D warnings

dev-env:
	rustup component add --toolchain $(shell cat rust-toolchain) rls rust-analysis rust-src rustc-dev clippy rustfmt

upgrade-filecoin-proofs:
	cargo update -p filecoin-proofs
	cargo update -p storage-proofs-core
	cargo update -p storage-proofs-porep
	cargo update -p storage-proofs-post
	cargo update -p storage-proofs-update
	cargo update -p filecoin-proofs-api
