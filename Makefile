all: build-smgr build-worker build-hwinfo

check-all: check-smgr check-worker check-hwinfo check-git

test-smgr:
	$(MAKE) -C ./venus-sector-manager/ test-all

build-smgr:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-sector-manager
	$(MAKE) -C ./venus-sector-manager/ build-all
	mv ./venus-sector-manager/venus-sector-manager ./dist/bin/

check-smgr:
	$(MAKE) -C ./venus-sector-manager/ check-all

test-worker:
	$(MAKE) -C ./venus-worker/ test-all

build-worker:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-worker
	$(MAKE) -C ./venus-worker/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./venus-worker/Cargo.toml | jq -r ".target_directory")/release/venus-worker ./dist/bin/

check-worker:
	$(MAKE) -C ./venus-worker/ check-all

build-hwinfo:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/hwinfo
	$(MAKE) -C ./hwinfo/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./hwinfo/Cargo.toml | jq -r ".target_directory")/release/hwinfo ./dist/bin/

check-hwinfo:
	$(MAKE) -C ./hwinfo/ check-all

check-git:
	./scripts/check-git-dirty.sh

clean:
	$(MAKE) -C ./venus-sector-manager/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

dev-env:
	ln -s ../../.githooks/pre-push ./.git/hooks/pre-push
	$(MAKE) -C ./venus-sector-manager/ dev-env
	$(MAKE) -C ./venus-worker/ dev-env
	$(MAKE) -C ./hwinfo/ dev-env
