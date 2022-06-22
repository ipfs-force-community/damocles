package internal

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/urfave/cli/v2"
)

var utilSealerSnapCmd = &cli.Command{
	Name: "snap",
	Subcommands: []*cli.Command{
		utilSealerSnapFetchCmd,
		utilSealerSnapCandidatesCmd,
		utilSealerSnapCancelCommitmentCmd,
	},
}

var utilSealerSnapFetchCmd = &cli.Command{
	Name:      "fetch",
	Usage:     "fetch acivated cc sectors in specified deadline as snap candidates",
	ArgsUsage: "<miner actor id/addr> <deadline index>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}

		mid, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return fmt.Errorf("parse miner actor: %w", err)
		}

		deadidx, err := strconv.ParseUint(args.Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("parse deadline index: %w", err)
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("extract api: %w", err)
		}

		defer stop()

		res, err := api.Sealer.SnapUpPreFetch(gctx, mid, &deadidx)
		if err != nil {
			return RPCCallError("SnapPreFetch", err)
		}

		Log.Infow("cadidate sectors fetched", "available-in-deadline", res.Total, "added", res.Diff)

		return nil
	},
}

var utilSealerSnapCandidatesCmd = &cli.Command{
	Name:  "candidates",
	Usage: "show fetched cc sectors for specified miner",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-all",
			Usage: "show all deadlines",
		},
	},
	ArgsUsage: "<miner actor id/addr>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}

		mid, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return fmt.Errorf("parse miner actor: %w", err)
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("extract api: %w", err)
		}

		defer stop()

		candidates, err := api.Sealer.SnapUpCandidates(gctx, mid)
		if err != nil {
			return RPCCallError("SnapPreFetch", err)
		}

		if len(candidates) == 0 {
			fmt.Fprintln(os.Stdout, "no candidates available")
			return nil
		}

		showAll := cctx.Bool("show-all")

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		defer tw.Flush()

		fmt.Fprintln(tw, "deadline\tcount")
		for i, bits := range candidates {
			var count uint64
			if bits != nil {
				count, err = bits.Count()
				if err != nil {
					return fmt.Errorf("failed to get count for deadline %d: %w", i, err)
				}
			}

			if count > 0 || showAll {
				fmt.Fprintf(tw, "%d\t%d\n", i, count)
			}
		}

		return nil
	},
}

var utilSealerSnapCancelCommitmentCmd = &cli.Command{
	Name:      "cancel-commit",
	Usage:     "cancel inflight snapup commitment",
	ArgsUsage: "<miner actor id/addr>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}

		mid, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return fmt.Errorf("parse miner actor: %w", err)
		}

		num, err := ShouldSectorNumber(args.Get(1))
		if err != nil {
			return fmt.Errorf("parse sector number: %w", err)
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("extract api: %w", err)
		}

		defer stop()

		err = api.Sealer.SnapUpCancelCommitment(gctx, abi.SectorID{
			Miner:  mid,
			Number: num,
		})

		if err != nil {
			return RPCCallError("CancelCommitment", err)
		}

		return nil
	},
}
