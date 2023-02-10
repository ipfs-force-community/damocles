package internal

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/venus-shared/types/messager"
)

var utilMessageCmd = &cli.Command{
	Name:  "message",
	Usage: "Track message",
	Subcommands: []*cli.Command{
		utilMessageWaitCmd,
		utilMessageSearchCmd,
	},
}

var utilMessageWaitCmd = &cli.Command{
	Name:      "wait",
	Usage:     "Wait for the execution result of the specified uid message",
	ArgsUsage: "<uid from venus-messager>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("build apis: %w", err)
		}

		defer stop()

		mid := cctx.Args().Get(0)
		msg, err := api.Messager.WaitMessage(gctx, mid, constants.MessageConfidence)
		if err != nil {
			return err
		}

		fmt.Println("State:", msg.State.String())
		if msg.State == messager.OnChainMsg || msg.State == messager.ReplacedMsg {
			fmt.Println("Cid:", msg.SignedCid.String())
			fmt.Println("Height:", msg.Height)
			fmt.Println("Tipset:", msg.TipSetKey.String())
			fmt.Println("ExitCode:", msg.Receipt.ExitCode)
			fmt.Println("gas_used:", msg.Receipt.GasUsed)
		}
		return nil
	},
}

var utilMessageSearchCmd = &cli.Command{
	Name:      "search",
	Usage:     "Search for the execution result of the specified uid message",
	ArgsUsage: "<uid from venus-messager>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			cli.ShowSubcommandHelpAndExit(cctx, 1)
			return nil
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("build apis: %w", err)
		}

		defer stop()

		mid := cctx.Args().Get(0)
		msg, err := api.Messager.GetMessageByUid(gctx, mid)
		if err != nil {
			return err
		}

		fmt.Println("State:", msg.State.String())
		if msg.State == messager.OnChainMsg || msg.State == messager.ReplacedMsg {
			fmt.Println("Cid:", msg.SignedCid.String())
			fmt.Println("Height:", msg.Height)
			fmt.Println("Tipset:", msg.TipSetKey.String())
			fmt.Println("ExitCode:", msg.Receipt.ExitCode)
			fmt.Println("gas_used:", msg.Receipt.GasUsed)
		}

		return nil
	},
}
