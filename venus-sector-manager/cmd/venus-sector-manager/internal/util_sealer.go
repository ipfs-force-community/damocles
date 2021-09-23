package internal

import (
	"context"
	"fmt"
	"os"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/dep"
)

func extractSealerClient(cctx *cli.Context) (api.SealerClient, context.Context, stopper, error) {
	gctx, gcancel := NewSigContext(cctx.Context)

	var sapi api.SealerClient

	stopper, err := dix.New(
		gctx,
		DepsFromCLICtx(cctx),
		dix.Override(new(dep.GlobalContext), gctx),
		dix.Override(new(dep.ListenAddress), dep.ListenAddress(cctx.String(SealerListenFlag.Name))),
		dep.SealerClient(&sapi),
	)

	if err != nil {
		gcancel()
		return sapi, nil, nil, fmt.Errorf("construct sealer api: %w", err)
	}

	return sapi, gctx, func() {
		stopper(cctx.Context)
		gcancel()
	}, nil
}

var utilSealerCmd = &cli.Command{
	Name: "sealer",
	Flags: []cli.Flag{
		SealerListenFlag,
	},
	Subcommands: []*cli.Command{
		utilSealerSectorsCmd,
	},
}

var utilSealerSectorsCmd = &cli.Command{
	Name: "sectors",
	Subcommands: []*cli.Command{
		utilSealerSectorsWorkerStatesCmd,
	},
}

var utilSealerSectorsWorkerStatesCmd = &cli.Command{
	Name: "worker-states",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractSealerClient(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.ListSectors(gctx)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "Sectors(%d):\n", len(states))
		for _, state := range states {
			fmt.Fprintf(os.Stdout, "m-%d-s-%d:\n", state.ID.Miner, state.ID.Number)
			if state.LatestState == nil {
				fmt.Fprintln(os.Stdout, "NULL")
				continue
			}

			fmt.Fprintln(os.Stdout, "\tWorker:")
			fmt.Fprintf(os.Stdout, "\t\tInstance: %s\n", state.LatestState.Worker.Instance)
			fmt.Fprintf(os.Stdout, "\t\tLocation: %s\n", state.LatestState.Worker.Location)

			fmt.Fprintln(os.Stdout, "\tState:")
			fmt.Fprintf(os.Stdout, "\t\tPrev: %s\n", state.LatestState.StateChange.Prev)
			fmt.Fprintf(os.Stdout, "\t\tCurrent: %s\n", state.LatestState.StateChange.Next)
			fmt.Fprintf(os.Stdout, "\t\tEvent: %s\n", state.LatestState.StateChange.Event)

			fmt.Fprintln(os.Stdout, "\tFailure:")
			if state.LatestState.Failure == nil {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			} else {
				fmt.Fprintf(os.Stdout, "\t\tLevel: %s\n", state.LatestState.Failure.Level)
				fmt.Fprintf(os.Stdout, "\t\tDesc: %s\n", state.LatestState.Failure.Desc)
			}

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}
