package main

import (
	_ "net/http/pprof"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/internal"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/ver"
)

var log = internal.Log

func main() {
	logging.Setup()

	app := &cli.App{
		Name:    "venus-sector-manager",
		Version: ver.VersionStr(),
		Commands: []*cli.Command{
			mockCmd,
			daemonCmd,
			internal.UtilCmd,
		},
		Flags: []cli.Flag{
			internal.HomeFlag,
			internal.NetFlag,
		},
		Before: func(cctx *cli.Context) error {
			if cctx.String(internal.NetFlag.Name) != "" {
				log.Warnf("DEPRECATED: the '%s' flag is deprecated", internal.NetFlag.Name)
			}
			return nil
		},
	}

	internal.RunApp(app)
}
