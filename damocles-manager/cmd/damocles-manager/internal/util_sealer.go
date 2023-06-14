package internal

import "github.com/urfave/cli/v2"

var utilSealerCmd = &cli.Command{
	Name:  "sealer",
	Usage: "Commands for sector sealing",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		utilSealerSectorsCmd,
		utilSealerProvingCmd,
		utilSealerActorCmd,
		utilSealerSnapCmd,
	},
}
