package main

import (
	"fmt"

	"github.com/docker/go-units"
	"github.com/dtynn/dix"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/cmd/damocles-manager/internal"
	"github.com/ipfs-force-community/damocles/damocles-manager/dep"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
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

		proofType, err := util.SectorSize2SealProofType(abi.SectorSize(sectorSize))
		if err != nil {
			return fmt.Errorf("get seal proof type: %w", err)
		}

		gctx, gcancel := internal.NewSigContext(cctx.Context)
		defer gcancel()

		var apiService *APIService
		stopper, err := dix.New(
			gctx,
			dix.Override(new(dep.GlobalContext), gctx),
			dix.Override(new(abi.ActorID), abi.ActorID(cctx.Uint64("miner"))),
			dix.Override(new(abi.RegisteredSealProof), proofType),
			dep.Mock(),
			dep.MockSealer(&apiService),
		)
		if err != nil {
			return fmt.Errorf("construct mock api: %w", err)
		}

		return serveAPI(gctx, stopper, apiService, cctx.String("listen"))
	},
}
