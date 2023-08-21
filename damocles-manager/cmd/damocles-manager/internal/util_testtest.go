package internal

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/randomness"
	"github.com/urfave/cli/v2"
)

var utilTestCmd = &cli.Command{
	Name:  "test",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		utilTestTestCmd,
	},
}

var utilTestTestCmd = &cli.Command{
	Name: "test",
	Action: func(cctx *cli.Context) error {
		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()
		rand, err := randomness.New(a.Chain)
		if err != nil {
			panic(err)
		}
		ts, err := a.Chain.ChainHead(actx)
		if err != nil {
			panic(err)
		}

		for i := 0; i < 10; i++ {
			tk, err := rand.GetTicket(actx, ts.Key(), ts.Height(), abi.ActorID(4040))
			if err != nil {
				panic(err)
			}

			fmt.Printf("%s: %s\n", tk.Ticket, tk.Epoch)
		}
		return nil
	},
}
