package main

import (
	_ "net/http/pprof"

	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/internal"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sector-manager/modules/policy"
)

var log = internal.Log

func main() {
	logging.Setup()

	app := &cli.App{
		Name: "venus-sector-manager",
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
			return policy.SetupNetwork(cctx.String(internal.NetFlag.Name))
		},
	}

	internal.RunApp(app)
}
