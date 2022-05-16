package main

import (
	"fmt"
	_ "net/http/pprof"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/internal"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/processor"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/ver"
)

var log = internal.Log

func main() {
	logging.Setup()

	app := &cli.App{
		Name:    "venus-sector-manager",
		Version: fmt.Sprintf("v%s-%s-%s", ver.Version, ver.Prover, ver.Commit),
		Commands: []*cli.Command{
			mockCmd,
			daemonCmd,
			internal.UtilCmd,
			processor.ProcessorCmd,
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
