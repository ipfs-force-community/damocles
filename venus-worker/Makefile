GIT_COMMIT=git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))

all: fmt clippy build-all

check-all: check-fmt check-clippy

build-all:
	GIT_COMMIT=$(GIT_COMMIT) cargo build --release

fmt:
	cargo fmt --all

check-fmt:
	cargo fmt --all -- --check

clippy:
	cargo clippy --fix --no-deps

check-clippy:
	cargo clippy --no-deps -- -D warnings
