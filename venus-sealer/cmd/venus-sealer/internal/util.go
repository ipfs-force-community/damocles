package internal

import "github.com/urfave/cli/v2"

var UtilCmd = &cli.Command{
	Name: "util",
	Subcommands: []*cli.Command{
		utilChainCmd,
	},
}
