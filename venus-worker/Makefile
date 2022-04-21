all: fmt clippy build-all

build-all:
	cargo build --release

fmt:
	cargo fmt --all

fmt-check:
	cargo fmt --all -- --check

clippy:
	cargo clippy --fix

clippy-check:
	cargo clippy -- -D warnings
