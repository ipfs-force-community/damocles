module github.com/dtynn/venus-cluster/venus-sector-manager

go 1.16

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/docker/go-units v0.4.0
	github.com/dtynn/dix v0.1.1
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-fil-commcid v0.0.0-20201016201715-d41df56b4f6a
	github.com/filecoin-project/go-fil-markets v1.2.5 // indirect
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/specs-actors v0.9.14
	github.com/filecoin-project/specs-actors/v2 v2.3.5
	github.com/filecoin-project/specs-actors/v5 v5.0.1
	github.com/filecoin-project/specs-storage v0.1.1-0.20201105051918-5188d9774506 // indirect
	github.com/filecoin-project/venus v1.0.2
	github.com/filecoin-project/venus-messager v1.1.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/ipfs-force-community/venus-common-utils v0.0.0-20210714054928-2042a9040759
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-ipfs-blockstore v1.0.3
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/libp2p/go-libp2p-core v0.7.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-multiaddr v0.3.3 // indirect
	github.com/multiformats/go-multihash v0.0.14
	github.com/urfave/cli/v2 v2.3.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20210219115102-f37d292932f2
	go.uber.org/fx v1.13.1
	go.uber.org/zap v1.16.0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

replace (
	github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
	github.com/filecoin-project/go-jsonrpc => github.com/ipfs-force-community/go-jsonrpc v0.1.4-0.20210721095535-a67dff16de21
	github.com/ipfs/go-ipfs-cmds => github.com/ipfs-force-community/go-ipfs-cmds v0.6.1-0.20210521090123-4587df7fa0ab
)
