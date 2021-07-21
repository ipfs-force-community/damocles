module github.com/dtynn/venus-cluster/venus-sealer

go 1.16

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/docker/go-units v0.4.0
	github.com/dtynn/dix v0.1.1
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/venus v1.0.2
	github.com/fsnotify/fsnotify v1.4.9
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-log/v2 v2.1.3
	github.com/mitchellh/go-homedir v1.1.0
	github.com/urfave/cli/v2 v2.3.0
	go.uber.org/fx v1.13.1
	go.uber.org/zap v1.16.0
)

replace github.com/ipfs/go-ipfs-cmds => github.com/ipfs-force-community/go-ipfs-cmds v0.6.1-0.20210521090123-4587df7fa0ab
