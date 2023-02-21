package main

import (
	"log"
	"os"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/plugin/internal"
	"github.com/urfave/cli/v2"
)

func main() {

	app := &cli.App{
		Name: "Tools for vsm-plugin",
		Commands: []*cli.Command{
			internal.BuildCmd,
			internal.CheckDepCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
