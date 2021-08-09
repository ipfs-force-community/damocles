all: build-sealer build-worker

build-sealer:
	rm -rf ./dist/bin/venus-sealer
	mkdir -p ./dist/bin/
	$(MAKE) -C ./venus-sealer/ build-all
	mv ./venus-sealer/venus-sealer ./dist/bin/

build-worker:
	cargo build --release --manifest-path=./venus-worker/Cargo.toml

claen:
	$(MAKE) -C ./venus-sealer/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean
