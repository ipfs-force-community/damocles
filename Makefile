all: build-smgr build-worker

build-smgr:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-sector-manager
	$(MAKE) -C ./venus-sector-manager/ build-all
	mv ./venus-sector-manager/venus-sector-manager ./dist/bin/

build-worker:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-worker
	$(MAKE) -C ./venus-worker/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./venus-worker/Cargo.toml | jq -r ".target_directory")/release/venus-worker ./dist/bin/

clean:
	$(MAKE) -C ./venus-sector-manager/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean
