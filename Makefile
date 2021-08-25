all: build-smgr build-worker

build-smgr:
	rm -rf ./dist/bin/venus-sector-manager
	mkdir -p ./dist/bin/
	$(MAKE) -C ./venus-sector-manager/ build-all
	mv ./venus-sector-manager/venus-sector-manager ./dist/bin/

build-worker:
	cargo build --release --manifest-path=./venus-worker/Cargo.toml

claen:
	$(MAKE) -C ./venus-sector-manager/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean
