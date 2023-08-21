package internal

import (
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var UtilCmd = &cli.Command{
	Name:  "util",
	Usage: "Commandline tools for damocles-manager",
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
		utilTestCmd,
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
