all: build-smgr build-worker build-worker-util

check-all: check-smgr check-worker check-worker-util check-git

test-smgr:
	$(MAKE) -C ./venus-sector-manager/ test-all

build-smgr:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-sector-manager
	$(MAKE) -C ./venus-sector-manager/ build-all
	mv ./venus-sector-manager/venus-sector-manager ./dist/bin/
#	mv ./venus-sector-manager/plugin-fsstore.so ./dist/bin/
#	mv ./venus-sector-manager/plugin-memdb.so ./dist/bin/

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

test-worker-util:
	$(MAKE) -C ./venus-worker-util/ test-all

build-worker-util:
	mkdir -p ./dist/bin/
	rm -rf ./dist/bin/venus-worker-util
	$(MAKE) -C ./venus-worker-util/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./venus-worker-util/Cargo.toml | jq -r ".target_directory")/release/venus-worker-util ./dist/bin/

check-worker-util:
	$(MAKE) -C ./venus-worker-util/ check-all

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
	$(MAKE) -C ./venus-worker-util/ dev-env

docker-smgr:
	docker build \
		-f Dockerfile.vsm \
		-t venus-sector-manager \
		--build-arg HTTPS_PROXY=${BUILD_DOCKER_PROXY} \
		--build-arg BUILD_TARGET=venus-sector-manager \
		.

docker-worker:
	$(MAKE) -C ./venus-worker/ docker


docker: docker-smgr docker-worker

TAG:=test
docker-push: docker
	docker tag venus-sector-manager $(PRIVATE_REGISTRY)/filvenus/venus-sector-manager:$(TAG)
	docker push $(PRIVATE_REGISTRY)/filvenus/venus-sector-manager:$(TAG)
	docker tag venus-worker $(PRIVATE_REGISTRY)/filvenus/venus-worker:$(TAG)
	docker push $(PRIVATE_REGISTRY)/filvenus/venus-worker:$(TAG)
