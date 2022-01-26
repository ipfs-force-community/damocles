module github.com/ipfs-force-community/venus-cluster/venus-sector-manager

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.3
	github.com/docker/go-units v0.4.0
	github.com/dtynn/dix v0.1.1
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/filecoin-project/go-state-types v0.1.3
	github.com/filecoin-project/specs-actors v0.9.14
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v5 v5.0.4
	github.com/filecoin-project/specs-actors/v6 v6.0.1
	github.com/filecoin-project/specs-actors/v7 v7.0.0-rc1
	github.com/filecoin-project/specs-storage v0.1.1-0.20211228030229-6d460d25a0c9
	github.com/filecoin-project/venus v1.2.0-rc4.0.20220126012146-002ee9b8e171
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/golang/snappy v0.0.3 // indirect
	github.com/hashicorp/go-multierror v1.1.1
	github.com/ipfs-force-community/venus-common-utils v0.0.0-20211122032945-eb6cab79c62a
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ipfs-blockstore v1.1.2
	github.com/ipfs/go-log/v2 v2.4.0
	github.com/libp2p/go-libp2p-core v0.13.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-multiaddr v0.4.1
	github.com/multiformats/go-multihash v0.1.0
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/urfave/cli/v2 v2.3.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20210713220151-be142a5ae1a8
	go.uber.org/fx v1.15.0
	go.uber.org/zap v1.19.1
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519 // indirect
	golang.org/x/mod v0.5.0 // indirect
	golang.org/x/sys v0.0.0-20211013075003-97ac67df715c // indirect
	golang.org/x/tools v0.1.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/genproto v0.0.0-20210828152312-66f60bf46e71 // indirect
)

replace (
	github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
	github.com/filecoin-project/go-jsonrpc => github.com/ipfs-force-community/go-jsonrpc v0.1.4-0.20210721095535-a67dff16de21
	github.com/ipfs/go-ipfs-cmds => github.com/ipfs-force-community/go-ipfs-cmds v0.6.1-0.20210521090123-4587df7fa0ab
)
