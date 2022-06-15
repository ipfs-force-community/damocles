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

		ps, err := assets.GetProofParams()
		if err != nil {
			return fmt.Errorf("get content of proof-params: %w", err)
		}

		srs, err := assets.GetSrs()
		if err != nil {
			return fmt.Errorf("get content of srs: %w", err)
		}

		err = paramfetch.GetParams(cctx.Context, ps, srs, sectorSize)
		if err != nil {
			return fmt.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
