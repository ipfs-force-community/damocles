module github.com/ipfs-force-community/venus-cluster/venus-sector-manager

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/dgraph-io/badger/v2 v2.2007.3
	github.com/docker/go-units v0.4.0
	github.com/dtynn/dix v0.1.2
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-commp-utils v0.1.3
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/filecoin-project/go-state-types v0.1.3
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v7 v7.0.0-rc1
	github.com/filecoin-project/specs-storage v0.2.0
	github.com/filecoin-project/venus v1.2.4-0.20220404074220-277186d62e53
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/hashicorp/go-multierror v1.1.1
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ipfs-blockstore v1.1.2
	github.com/ipfs/go-ipld-cbor v0.0.6
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
	golang.org/x/tools v0.1.7 // indirect
)

replace (
	github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
	github.com/filecoin-project/go-jsonrpc => github.com/ipfs-force-community/go-jsonrpc v0.1.4-0.20210721095535-a67dff16de21
	github.com/ipfs/go-ipfs-cmds => github.com/ipfs-force-community/go-ipfs-cmds v0.6.1-0.20210521090123-4587df7fa0ab
)
