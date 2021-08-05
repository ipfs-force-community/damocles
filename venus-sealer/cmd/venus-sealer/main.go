package main

import (
	_ "net/http/pprof"

	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/cmd/venus-sealer/internal"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
)

var log = internal.Log

func main() {
	logging.Setup()

	app := &cli.App{
		Name: "venus-sealer",
		Commands: []*cli.Command{
			mockCmd,
			daemonCmd,
			internal.UtilCmd,
		},
		Flags: []cli.Flag{
			internal.HomeFlag,
		},
	}

	internal.RunApp(app)
}
