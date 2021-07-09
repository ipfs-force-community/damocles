package main

import "github.com/urfave/cli/v2"

var daemonCmd = &cli.Command{
	Name: "daemon",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "home",
			Value: "~/.venus-sealer",
		},
	},
	Action: func(cctx *cli.Context) error {
		return nil
	},
}
