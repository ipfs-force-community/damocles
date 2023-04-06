all: build-manager build-worker build-worker-util

check-all: check-manager check-worker check-worker-util check-git

test-manager:
	$(MAKE) -C ./damocles-manager/ test-all

build-manager:
	mkdir ./dist/
	rm -rf ./dist/damocles-manager
	$(MAKE) -C ./damocles-manager/ build-all
	mv ./damocles-manager/damocles-manager ./dist/
#	mv ./damocles-manager/plugin-fsstore.so ./dist/
#	mv ./damocles-manager/plugin-memdb.so ./dist/

check-manager:
	$(MAKE) -C ./damocles-manager/ check-all

test-worker:
	$(MAKE) -C ./damocles-worker/ test-all

build-worker:
	mkdir ./dist/
	rm -rf ./dist/damocles-worker
	$(MAKE) -C ./damocles-worker/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./damocles-worker/Cargo.toml | jq -r ".target_directory")/release/damocles-worker ./dist/

check-worker:
	$(MAKE) -C ./damocles-worker/ check-all

test-worker-util:
	$(MAKE) -C ./damocles-worker-util/ test-all

build-worker-util:
	mkdir ./dist/
	rm -rf ./dist/damocles-worker-util
	$(MAKE) -C ./damocles-worker-util/ build-all
	cp $(shell cargo metadata --format-version=1 --manifest-path=./damocles-worker-util/Cargo.toml | jq -r ".target_directory")/release/damocles-worker-util ./dist/

check-worker-util:
	$(MAKE) -C ./damocles-worker-util/ check-all

check-git:
	./scripts/check-git-dirty.sh

clean:
	$(MAKE) -C ./damocles-manager/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

dev-env:
	ln -s ../../.githooks/pre-push ./.git/hooks/pre-push
	$(MAKE) -C ./damocles-manager/ dev-env
	$(MAKE) -C ./damocles-worker/ dev-env
	$(MAKE) -C ./damocles-worker-util/ dev-env

docker-smgr:
	docker build \
		-f Dockerfile.manager \
		-t damocles-manager \
		--build-arg HTTPS_PROXY=${BUILD_DOCKER_PROXY} \
		--build-arg BUILD_TARGET=damocles-manager \
		.

docker-worker:
	$(MAKE) -C ./damocles-worker/ docker


docker: docker-smgr docker-worker

TAG:=test
docker-push: docker
	docker tag damocles-manager $(PRIVATE_REGISTRY)/filvenus/damocles-manager:$(TAG)
	docker push $(PRIVATE_REGISTRY)/filvenus/damocles-manager:$(TAG)
	docker tag damocles-worker $(PRIVATE_REGISTRY)/filvenus/damocles-worker:$(TAG)
	docker push $(PRIVATE_REGISTRY)/filvenus/damocles-worker:$(TAG)
