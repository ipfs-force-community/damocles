all: fmt clippy build-all

check-all: check-fmt check-clippy

build-all:
	cargo build --release

fmt:
	cargo fmt --all

check-fmt:
	cargo fmt --all -- --check

clippy:
	cargo clippy --fix

check-clippy:
	cargo clippy -- -D warnings
