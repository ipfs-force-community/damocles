package main

import (
	"log"
	"os"

	"github.com/ipfs-force-community/damocles/damocles-manager/cmd/plugin/internal"
	"github.com/urfave/cli/v2"
)

func main() {

	app := &cli.App{
		Name: "Tools for damocles-manager-plugin",
		Commands: []*cli.Command{
			internal.BuildCmd,
			internal.CheckDepCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
