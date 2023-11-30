GO?=go

build-all: build-concurrent-limiter

build-concurrent-limiter:
	rm -rf ./plugin-concurrent-limiter.so
	$(GO) run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- build --src-dir=./ --out-dir=./

check-dep:
	$(GO) run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- check-dep --goc=$(GO) go.mod

fix-dep:
	$(GO) run github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin@latest -- check-dep --fix --goc=$(GO) go.mod
