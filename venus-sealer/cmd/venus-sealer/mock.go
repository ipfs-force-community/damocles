package main

import (
	"fmt"

	"github.com/docker/go-units"
	"github.com/dtynn/dix"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/dep"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/util"
)

var mockCmd = &cli.Command{
	Name: "mock",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "miner",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "sector-size",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "listen",
			Value: ":1789",
		},
	},
	Action: func(cctx *cli.Context) error {
		sizeStr := cctx.String("sector-size")
		sectorSize, err := units.RAMInBytes(sizeStr)
		if err != nil {
			return fmt.Errorf("invalid sector-size string %s: %w", sizeStr, err)
		}

		proofType, err := util.SectorSize2SealProofType(uint64(sectorSize))
		if err != nil {
			return fmt.Errorf("get seal proof type: %w", err)
		}

		var node api.SealerAPI
		stopper, err := dix.New(
			cctx.Context,
			dix.Override(new(dep.GlobalContext), cctx.Context),
			dix.Override(new(abi.ActorID), abi.ActorID(cctx.Uint64("miner"))),
			dix.Override(new(abi.RegisteredSealProof), proofType),
			dep.Mock(),
			dep.Sealer(&node),
		)
		if err != nil {
			return fmt.Errorf("construct mock api: %w", err)
		}

		defer stopper(cctx.Context)

		return serveSealerAPI(node, cctx.String("listen"))
	},
}
