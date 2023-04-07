package internal

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

var utilMarketCmd = &cli.Command{
	Name:  "market",
	Usage: "Inspect the storage market actor",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		utilMarketReleaseDealsCmd,
	},
}

var utilMarketReleaseDealsCmd = &cli.Command{
	Name:  "release-deals",
	Usage: "Manual release deals",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "miner",
			Required: true,
		},
		&cli.Int64SliceFlag{
			Name:    "deals",
			Aliases: []string{"deal"},
		},
	},
	Action: func(cctx *cli.Context) error {
		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		mid := cctx.Uint64("miner")
		maddr, err := address.NewIDAddress(mid)
		if err != nil {
			return fmt.Errorf("invalid miner actor id %d: %w", mid, err)
		}
		dealsI64 := cctx.Int64Slice("deals")
		deals := make([]abi.DealID, len(dealsI64))
		for i, dealI64 := range dealsI64 {
			deals[i] = abi.DealID(dealI64)
		}
		if err := api.Market.ReleaseDeals(gctx, maddr, deals); err != nil {
			return fmt.Errorf("failed to release deals: %w", err)
		}
		return nil
	},
}
