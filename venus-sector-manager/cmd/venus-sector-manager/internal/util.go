package internal

import (
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var UtilCmd = &cli.Command{
	Name:  "util",
	Usage: "Commandline tools for venus-sector-manager",
	Subcommands: []*cli.Command{
		utilChainCmd,
		utilMinerCmd,
		utilSealerCmd,
		utilMarketCmd,
		utilStorageCmd,
		utilWorkerCmd,
		utilMessageCmd,
		utilFetchParamCmd,
		utilMigrateCmd,
	},
	Flags: []cli.Flag{
		SealerListenFlag,
		ConfDirFlag,
	},
	Before: func(cctx *cli.Context) error {
		logging.SetupForSub(logSubSystem)
		return nil
	},
}
