export GIT_COMMIT=git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
export RUSTFLAGS

all: fmt clippy build-all

check-all: check-fmt check-clippy

# Add `--cfg unsound_local_offset` flag to allow time crate to get the local time zone. \
See: https://docs.rs/time/0.3.9/time/index.html#feature-flags and \
https://github.com/time-rs/time/issues/293#issuecomment-1005002386
build-all: RUSTFLAGS+=--cfg unsound_local_offset
build-all:
	cargo build --release

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
