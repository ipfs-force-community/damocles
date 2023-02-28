package internal

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
)

var utilMarketCmd = &cli.Command{
	Name:  "market",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		utilMarketReleaseDealsCmd,
	},
}

var utilMarketReleaseDealsCmd = &cli.Command{
	Name: "release-deals",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "miner",
			Required: true,
		},
		&cli.Int64SliceFlag{
			Name: "deal",
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

		mlog := Log.With("miner", maddr)

		for _, dealID := range cctx.Int64Slice("deal") {
			err = api.Market.UpdateDealStatus(gctx, maddr, abi.DealID(dealID), market.DealStatusUndefine, storagemarket.StorageDealAwaitingPreCommit)
			if err == nil {
				mlog.Infow("deal released", "deal-id", dealID)
			} else {
				mlog.Errorf("failed to release deal %d: %w", dealID, err)
			}
		}

		mlog.Info("all done")
		return nil
	},
}
