package main

import (
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/logging"
)

var log = logging.New("sealer")

func main() {
	logging.Setup()

	app := &cli.App{
		Name: "venus-sealer",
		Commands: []*cli.Command{
			mockCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("run app: %s", err)
	}
}
