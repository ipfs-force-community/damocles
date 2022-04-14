package internal

import (
	"fmt"
	"os"
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

var utilSealerCmd = &cli.Command{
	Name: "sealer",
	Flags: []cli.Flag{
		SealerListenFlag,
	},
	Subcommands: []*cli.Command{
		utilSealerSectorsCmd,
		utilSealerProvingCmd,
		utilSealerActorCmd,
		// utilSealerSnapCmd,
	},
}

var utilSealerSectorsCmd = &cli.Command{
	Name: "sectors",
	Subcommands: []*cli.Command{
		utilSealerSectorsWorkerStatesCmd,
		utilSealerSectorsAbortCmd,
		utilSealerSectorsListCmd,
		utilSealerSectorsRestoreCmd,
	},
}

var utilSealerSectorsWorkerStatesCmd = &cli.Command{
	Name: "worker-states",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, api.WorkerOnline)
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

var utilSealerSectorsAbortCmd = &cli.Command{
	Name:      "abort",
	ArgsUsage: "<miner actor> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.ReportAborted(gctx, abi.SectorID{
			Miner:  abi.ActorID(miner),
			Number: abi.SectorNumber(sectorNum),
		}, "aborted via CLI")
		if err != nil {
			return fmt.Errorf("abort sector failed: %w", err)
		}

		return nil
	},
}

var utilSealerSectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "Print sector data in completed state",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, api.WorkerOffline)
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

			fmt.Fprintln(os.Stdout, "\tDeals:")
			deals := state.Deals()
			if len(deals) == 0 {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			} else {
				for _, deal := range deals {
					fmt.Fprintf(os.Stdout, "\t\tID: %d\n", deal.ID)
					fmt.Fprintf(os.Stdout, "\t\tPiece: %v\n", deal.Piece)
				}
			}

			fmt.Fprintln(os.Stdout, "\tTicket:")
			if state.Ticket != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Ticket.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Ticket.Ticket)
			} else {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			}

			fmt.Fprintln(os.Stdout, "\tSeed:")
			if state.Seed != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Seed.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Seed.Seed)
			} else {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			}

			fmt.Fprintln(os.Stdout, "\tMessageInfo:")
			if state.MessageInfo.PreCommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tPre: %s\n", state.MessageInfo.PreCommitCid.String())
			} else {
				fmt.Fprintln(os.Stdout, "\t\tPre: NULL")
			}
			if state.MessageInfo.CommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tProve: %s\n", state.MessageInfo.CommitCid.String())
			} else {
				fmt.Fprintln(os.Stdout, "\t\tProve: NULL")
			}
			fmt.Fprintf(os.Stdout, "\t\tNeedSeed: %v\n", state.MessageInfo.NeedSend)

			fmt.Fprintln(os.Stdout, "\tState:")
			fmt.Fprintf(os.Stdout, "\t\tPrev: %s\n", state.LatestState.StateChange.Prev)
			fmt.Fprintf(os.Stdout, "\t\tCurrent: %s\n", state.LatestState.StateChange.Next)
			fmt.Fprintf(os.Stdout, "\t\tEvent: %s\n", state.LatestState.StateChange.Event)

			fmt.Fprintf(os.Stdout, "\tFinalized: %v\n", state.Finalized)

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}

var utilSealerSectorsRestoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "restore a sector state that may already finalized or aborted",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:   "force",
			Hidden: true,
			Value:  false,
		},
	},
	ArgsUsage: "<miner actor id> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.RestoreSector(gctx, abi.SectorID{
			Miner:  miner,
			Number: abi.SectorNumber(sectorNum),
		}, cctx.Bool("force"))
		if err != nil {
			return fmt.Errorf("restore sector failed: %w", err)
		}

		return nil
	},
}
