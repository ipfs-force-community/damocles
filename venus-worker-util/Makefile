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
