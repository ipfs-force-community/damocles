package main

import (
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/cmd/damocles-manager/internal"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/ver"
)

var log = internal.Log

func main() {
	logging.Setup()

	app := &cli.App{
		Name:    "damocles-manager",
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
