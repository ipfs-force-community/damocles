GO?=go

build:
	rm -rf ./plugin-memdb.so
	$(GO) run github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/plugin@latest -- build --out-dir=./ --src-dir=./

check-dep:
	$(GO) run github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/plugin@latest -- check-dep --goc=$(GO) go.mod

fix-dep:
	$(GO) run github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/plugin@latest -- check-dep --fix --goc=$(GO) go.mod
