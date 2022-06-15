package internal

import (
	"fmt"

	"github.com/docker/go-units"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-paramfetch"

	"github.com/filecoin-project/venus/fixtures/assets"
)

var utilFetchParamCmd = &cli.Command{
	Name:      "fetch-params",
	Usage:     "Fetch proving parameters",
	ArgsUsage: `<sectorSize (eg. "32GiB", "512MiB", ...)>`,
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}
		sectorSizeInt, err := units.RAMInBytes(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("error parsing sector size (specify as \"32GiB\", for instance): %w", err)
		}
		sectorSize := uint64(sectorSizeInt)

		ps, err := asset.Asset("fixtures/_assets/proof-params/parameters.json")
		if err != nil {
			return err
		}
		srs, err := asset.Asset("fixtures/_assets/proof-params/srs-inner-product.json")
		if err != nil {
			return err
		}

		err = paramfetch.GetParams(cctx.Context, ps, srs, sectorSize)
		if err != nil {
			return fmt.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
