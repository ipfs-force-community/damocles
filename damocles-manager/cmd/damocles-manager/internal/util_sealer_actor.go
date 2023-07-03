package internal

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/adt"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

var utilSealerActorCmd = &cli.Command{
	Name:  "actor",
	Usage: "Manipulate the miner actor",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "miner",
			Required: true,
			Usage:    "miner actor id/address",
		},
	},
	Subcommands: []*cli.Command{
		utilSealerActorBalanceCmd,
		utilSealerActorAddBalanceCmd,
		utilSealerActorWithdrawCmd,
		utilSealerActorRepayDebtCmd,
		utilSealerActorSetOwnerCmd,
		utilSealerActorControl,
		utilSealerActorProposeChangeWorker,
		utilSealerActorConfirmChangeWorker,
		utilSealerActorCompactAllocatedCmd,
		utilSealerActorProposeChangeBeneficiary,
		utilSealerActorConfirmChangeBeneficiary,
	},
}

var utilSealerActorBalanceCmd = &cli.Command{
	Name: "balance",
	Action: func(cctx *cli.Context) error {
		miner, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mlog := Log.With("miner", miner.String())

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("build apis: %w", err)
		}

		defer stop()

		ts, err := api.Chain.ChainHead(gctx)
		if err != nil {
			return RPCCallError("ChainHead", err)
		}

		mlog = mlog.With("height", ts.Height())

		actor, err := api.Chain.StateGetActor(gctx, miner, ts.Key())
		if err != nil {
			return RPCCallError("StateGetActor", err)
		}

		mlog.Infof("balance: %s", modules.FIL(actor.Balance).Short())

		return nil
	},
}

var utilSealerActorAddBalanceCmd = &cli.Command{
	Name: "add-balance",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "exid",
		},
	},
	ArgsUsage: "<from address> <amount>",
	Action: func(cctx *cli.Context) error {
		to, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		args := cctx.Args()
		if count := args.Len(); count < 2 {
			return fmt.Errorf("2 args required, got %d", count)
		}

		from, err := ShouldAddress(args.Get(0), true, false)
		if err != nil {
			return fmt.Errorf("parse from address: %w", err)
		}

		value, err := modules.ParseFIL(args.Get(1))
		if err != nil {
			return fmt.Errorf("parse value: %w", err)
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("build apis: %w", err)
		}

		defer stop()

		msg := &messager.UnsignedMessage{
			From: from,

			To:     to,
			Method: 0,
			Params: []byte{},

			Value: value.Std(),
		}

		err = waitMessage(gctx, api, msg, cctx.String("exid"), nil, nil)
		if err != nil {
			return err
		}

		return nil
	},
}

var utilSealerActorWithdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "Withdraw available balance",
	ArgsUsage: "[amount (FIL)]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(policy.InteractivePoRepConfidence),
		},
		&cli.BoolFlag{
			Name:  "beneficiary",
			Usage: "send withdraw message from the beneficiary address",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		available, err := api.Chain.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		amount := available
		if cctx.Args().Present() {
			f, err := types.ParseFIL(cctx.Args().First())
			if err != nil {
				return fmt.Errorf("parsing 'amount' argument: %w", err)
			}

			amount = abi.TokenAmount(f)

			if amount.GreaterThan(available) {
				return fmt.Errorf("can't withdraw more funds than available; requested: %s; available: %s", types.FIL(amount), types.FIL(available))
			}
		}

		params, err := actors.SerializeParams(&core.WithdrawBalanceParams{
			AmountRequested: amount, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			return err
		}

		var sender address.Address
		if cctx.IsSet("beneficiary") {
			sender = mi.Beneficiary
		} else {
			sender = mi.Owner
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			To:     maddr,
			From:   sender,
			Value:  types.NewInt(0),
			Method: stbuiltin.MethodsMiner.WithdrawBalance,
			Params: params,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", mid)

		// wait for it to get mined into a block
		fmt.Printf("waiting for %d epochs for confirmation..\n", uint64(cctx.Int("confidence")))

		wait, err := api.Messager.WaitMessage(ctx, mid, uint64(cctx.Int("confidence")))
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println(cctx.App.Writer, "withdrawal failed!")
			return err
		}

		nv, err := api.Chain.StateNetworkVersion(ctx, wait.TipSetKey)
		if err != nil {
			return err
		}

		if nv >= network.Version14 {
			var withdrawn abi.TokenAmount
			if err := withdrawn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
				return err
			}

			fmt.Printf("Successfully withdrew %s \n", types.FIL(withdrawn))
			if withdrawn.LessThan(amount) {
				fmt.Printf("Note that this is less than the requested amount of %s\n", types.FIL(amount))
			}
		}

		return nil
	},
}

var utilSealerActorRepayDebtCmd = &cli.Command{
	Name:      "repay-debt",
	Usage:     "Pay down a miner's debt",
	ArgsUsage: "[amount (FIL)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		var amount abi.TokenAmount
		if cctx.Args().Present() {
			f, err := types.ParseFIL(cctx.Args().First())
			if err != nil {
				return fmt.Errorf("parsing 'amount' argument: %w", err)
			}

			amount = abi.TokenAmount(f)
		} else {
			mact, err := api.Chain.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			store := adt.WrapStore(ctx, cbor.NewCborStore(chain.NewAPIBlockstore(api.Chain)))
			mst, err := miner.Load(store, mact)
			if err != nil {
				return err
			}

			amount, err = mst.FeeDebt()
			if err != nil {
				return err
			}
		}

		fromAddr := mi.Worker
		if from := cctx.String("from"); from != "" {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		fromID, err := api.Chain.StateLookupID(ctx, fromAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if !util.IsController(mi, fromID) {
			return fmt.Errorf("sender isn't a controller of miner: %s", fromID)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			To:     maddr,
			From:   fromID,
			Value:  amount,
			Method: stbuiltin.MethodsMiner.RepayDebt,
			Params: nil,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Sent repay debt message %s\n", mid)

		return nil
	},
}

var utilSealerActorControl = &cli.Command{
	Name:  "control",
	Usage: "Manage control addresses",
	Subcommands: []*cli.Command{
		utilSealerActorControlList,
		utilSealerActorControlSet,
	},
}

var utilSealerActorControlList = &cli.Command{
	Name:  "list",
	Usage: "Get currently set control addresses",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "verbose",
		},
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		minerInfo, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return fmt.Errorf("invalid miner addr '%s': %w", maddr, err)
		}
		minerConfig, err := api.Damocles.GetMinerConfig(ctx, abi.ActorID(mid))
		if err != nil {
			return fmt.Errorf("get miner config: %w", err)
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 10, 2, ' ', 0)
		defer tw.Flush()
		_, _ = fmt.Fprintln(tw, "name\tID\tkey\tuse\tbalance")

		compareAddr := func(a1 []address.Address, a2 address.Address) (bool, error) {
			if len(a1) == 0 {
				return false, nil
			}

			a1t0 := address.Undef
			for _, a1Item := range a1 {
				if a1Item.Protocol() == a2.Protocol() {
					return a1Item == a2, nil
				}
				if a1Item.Protocol() == address.ID {
					a1t0 = a1Item
				}
			}

			if a1t0.Empty() {
				var err error
				a1t0, err = api.Chain.StateLookupID(ctx, a1[0], types.EmptyTSK)
				if err != nil {
					return false, err
				}
			}

			a2t0, err := api.Chain.StateLookupID(ctx, a2, types.EmptyTSK)
			if err != nil {
				return false, err
			}

			return a1t0 == a2t0, nil
		}

		printKey := func(name string, addr address.Address) {
			var actor *types.Actor
			if actor, err = api.Chain.StateGetActor(ctx, addr, types.EmptyTSK); err != nil {
				fmt.Printf("%s\t%s: error getting actor: %s\n", name, addr, err)
				return
			}

			var k = addr
			// `a` maybe a `robust`, in that case, `StateAccountKey` returns an error.
			if builtin.IsAccountActor(actor.Code) {
				if k, err = api.Chain.StateAccountKey(ctx, addr, types.EmptyTSK); err != nil {
					fmt.Printf("%s\t%s: error getting account key: %s\n", name, addr, err)
					return
				}
			}
			kstr := k.String()
			if !cctx.Bool("verbose") {
				if len(kstr) > 9 {
					kstr = kstr[:6] + "..."
				}
			}

			balance := types.FIL(actor.Balance).String()
			switch {
			case actor.Balance.LessThan(types.FromFil(10)):
				balance = color.RedString(balance)
			case actor.Balance.LessThan(types.FromFil(50)):
				balance = color.YellowString(balance)
			default:
				balance = color.GreenString(balance)
			}

			var uses []string
			if eq, err := compareAddr([]address.Address{addr, k}, minerInfo.Worker); err == nil && eq {
				uses = append(uses, "other")
			}
			if eq, err := compareAddr([]address.Address{addr, k}, minerConfig.Commitment.Pre.Sender.Std()); err == nil && eq {
				uses = append(uses, "precommit")
			}
			if eq, err := compareAddr([]address.Address{addr, k}, minerConfig.Commitment.Prove.Sender.Std()); err == nil && eq {
				uses = append(uses, "provecommit")
			}
			if eq, err := compareAddr([]address.Address{addr, k}, minerConfig.Commitment.Terminate.Sender.Std()); err == nil && eq {
				uses = append(uses, "terminate")
			}
			if eq, err := compareAddr([]address.Address{addr, k}, minerConfig.PoSt.Sender.Std()); err == nil && eq {
				uses = append(uses, "post")
			}
			if eq, err := compareAddr([]address.Address{addr, k}, minerConfig.SnapUp.Sender.Std()); err == nil && eq {
				uses = append(uses, "snapup")
			}

			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", name, addr, kstr, strings.Join(uses, " "), balance)
		}

		printKey("owner", minerInfo.Owner)
		printKey("worker", minerInfo.Worker)
		printKey("beneficiary", minerInfo.Beneficiary)
		for i, ca := range minerInfo.ControlAddresses {
			printKey(fmt.Sprintf("control-%d", i), ca)
		}

		return nil
	},
}

var utilSealerActorControlSet = &cli.Command{
	Name:      "set",
	Usage:     "Set control address(-es)",
	ArgsUsage: "[...address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		del := map[address.Address]struct{}{}
		existing := map[address.Address]struct{}{}
		for _, controlAddress := range mi.ControlAddresses {
			ka, err := api.Chain.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
			if err != nil {
				return err
			}

			del[ka] = struct{}{}
			existing[ka] = struct{}{}
		}

		var toSet []address.Address

		for i, as := range cctx.Args().Slice() {
			a, err := address.NewFromString(as)
			if err != nil {
				return fmt.Errorf("parsing address %d: %w", i, err)
			}

			ka, err := api.Chain.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				return err
			}

			// make sure the address exists on chain
			_, err = api.Chain.StateLookupID(ctx, ka, types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("looking up %s: %w", ka, err)
			}

			delete(del, ka)
			toSet = append(toSet, ka)
		}

		for a := range del {
			fmt.Println("Remove", a)
		}
		for _, a := range toSet {
			if _, exists := existing[a]; !exists {
				fmt.Println("Add", a)
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		cwp := &core.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ChangeWorkerAddress,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}

		fmt.Println("Message ID:", mid)

		return nil
	},
}

var utilSealerActorSetOwnerCmd = &cli.Command{
	Name:      "set-owner",
	Usage:     "Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)",
	ArgsUsage: "[newOwnerAddress senderAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		if cctx.NArg() != 2 {
			return fmt.Errorf("must pass new owner address and sender address")
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddrID, err := api.Chain.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		fa, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		fromAddrID, err := api.Chain.StateLookupID(ctx, fa, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if fromAddrID != mi.Owner && fromAddrID != newAddrID {
			return fmt.Errorf("from address must either be the old owner or the new owner")
		}

		sp, err := actors.SerializeParams(&newAddrID)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   fromAddrID,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ChangeOwnerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}

		fmt.Println("Message ID:", mid)

		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("owner change failed!")
			return err
		}

		fmt.Println("message succeeded!")

		return nil
	},
}

var utilSealerActorProposeChangeWorker = &cli.Command{
	Name:      "propose-change-worker",
	Usage:     "Propose a worker address change",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of new worker address")
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.Chain.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			if mi.Worker == newAddr {
				return fmt.Errorf("worker address already set to %s", na)
			}
		} else {
			if mi.NewWorker == newAddr {
				return fmt.Errorf("change to worker address %s already pending", na)
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		cwp := &core.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ChangeWorkerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}

		fmt.Println("Propose Message CID:", mid)

		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("Propose worker change failed!")
			return err
		}

		mi, err = api.Chain.StateMinerInfo(ctx, maddr, wait.TipSetKey)
		if err != nil {
			return err
		}
		if mi.NewWorker != newAddr {
			return fmt.Errorf("proposed worker address change not reflected on chain: expected %s, found %s", na, mi.NewWorker)
		}

		fmt.Printf("Worker key change to %s successfully proposed.\n", na)
		fmt.Printf("Call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)

		return nil
	},
}

var utilSealerActorConfirmChangeWorker = &cli.Command{
	Name:      "confirm-change-worker",
	Usage:     "Confirm a worker address change",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of new worker address")
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.Chain.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			return fmt.Errorf("no worker key change proposed")
		} else if mi.NewWorker != newAddr {
			return fmt.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker)
		}

		if head, err := api.Chain.ChainHead(ctx); err != nil {
			return fmt.Errorf("failed to get the chain head: %w", err)
		} else if head.Height() < mi.WorkerChangeEpoch {
			return fmt.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height())
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ConfirmChangeWorkerAddress,
			Value:  big.Zero(),
		}, nil)
		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}

		fmt.Println("Confirm Message ID:", mid)

		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("Worker change execute failed!")
			return err
		}

		mi, err = api.Chain.StateMinerInfo(ctx, maddr, wait.TipSetKey)
		if err != nil {
			return err
		}
		if mi.Worker != newAddr {
			return fmt.Errorf("confirmed worker address change not reflected on chain: expected '%s', found '%s'", newAddr, mi.Worker)
		}

		return nil
	},
}

var utilSealerActorCompactAllocatedCmd = &cli.Command{
	Name:  "compact-allocated",
	Usage: "Compact allocated sectors bitfield",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "mask-last-offset",
			Usage: "Mask sector IDs from 0 to 'higest_allocated - offset'",
		},
		&cli.Uint64Flag{
			Name:  "mask-upto-n",
			Usage: "Mask sector IDs from 0 to 'n'",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mact, err := api.Chain.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(chain.NewAPIBlockstore(api.Chain)))

		mst, err := miner.Load(store, mact)
		if err != nil {
			return err
		}

		allocs, err := mst.GetAllocatedSectors()
		if err != nil {
			return err
		}

		var maskBf bitfield.BitField

		{
			exclusiveFlags := []string{"mask-last-offset", "mask-upto-n"}
			hasFlag := false
			for _, f := range exclusiveFlags {
				if hasFlag && cctx.IsSet(f) {
					return fmt.Errorf("more than one 'mask` flag set")
				}
				hasFlag = hasFlag || cctx.IsSet(f)
			}
		}
		switch {
		case cctx.IsSet("mask-last-offset"):
			last, err := allocs.Last()
			if err != nil {
				return err
			}

			m := cctx.Uint64("mask-last-offset")
			if last <= m+1 {
				return fmt.Errorf("highest allocated sector lower than mask offset %d: %d", m+1, last)
			}
			// security to not brick a miner
			if last > 1<<60 {
				return fmt.Errorf("very high last sector number, refusing to mask: %d", last)
			}

			maskBf, err = bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{
				Runs: []rlepluslazy.Run{{Val: true, Len: last - m}}})
			if err != nil {
				return fmt.Errorf("forming bitfield: %w", err)
			}
		case cctx.IsSet("mask-upto-n"):
			n := cctx.Uint64("mask-upto-n")
			maskBf, err = bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{
				Runs: []rlepluslazy.Run{{Val: true, Len: n}}})
			if err != nil {
				return fmt.Errorf("forming bitfield: %w", err)
			}
		default:
			return fmt.Errorf("no 'mask' flags set")
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params := &core.CompactSectorNumbersParams{
			MaskSectorNumbers: maskBf,
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   mi.Worker,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.CompactSectorNumbers,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return fmt.Errorf("mpool push: %w", err)
		}

		fmt.Println("CompactSectorNumbers Message ID:", mid)

		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("Propose owner change execute failed")
			return err
		}

		return nil
	},
}

var utilSealerActorProposeChangeBeneficiary = &cli.Command{
	Name:      "propose-change-beneficiary",
	Usage:     "Propose a beneficiary address change",
	ArgsUsage: "[beneficiaryAddress quota expiration]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "overwrite-pending-change",
			Usage: "Overwrite the current beneficiary change proposal",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return fmt.Errorf("must past 3 args")
		}
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("parsing beneficiary address: %w", err)
		}

		newAddr, err := api.Chain.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("looking up new beneficiary address: %w", err)
		}

		quota, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("parsing quota: %w", err)
		}

		expiration, err := strconv.ParseInt(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return fmt.Errorf("parsing expiration: %w", err)
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		if mi.Beneficiary == mi.Owner && newAddr == mi.Owner {
			return fmt.Errorf("beneficiary %s already set to owner address", mi.Beneficiary)
		}

		if mi.PendingBeneficiaryTerm != nil {
			fmt.Println("WARNING: replacing Pending Beneficiary Term of:")
			fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
			fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
			fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)

			if !cctx.Bool("overwrite-pending-change") {
				return fmt.Errorf("must pass --overwrite-pending-change to replace current pending beneficiary change. Please review CAREFULLY")
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
			return nil
		}

		params := &types.ChangeBeneficiaryParams{
			NewBeneficiary: newAddr,
			NewQuota:       abi.TokenAmount(quota),
			NewExpiration:  abi.ChainEpoch(expiration),
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ChangeBeneficiary,
			Value:  big.Zero(),
			Params: sp,
		}, nil)

		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}
		fmt.Println("Propose Message ID:", mid)
		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return fmt.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("propose beneficiary change failed")
		}

		updatedMinerInfo, err := api.Chain.StateMinerInfo(ctx, maddr, wait.TipSetKey)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == newAddr {
			fmt.Println("Beneficiary address successfully changed")
		} else {
			fmt.Println("Beneficiary address change awaiting additional confirmations")
		}
		return nil
	},
}
var utilSealerActorConfirmChangeBeneficiary = &cli.Command{
	Name:  "confirm-change-beneficiary",
	Usage: "Confirm a beneficiary address change",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "existing-beneficiary",
			Usage: "send confirmation from the existing beneficiary address",
		},
		&cli.BoolFlag{
			Name:  "new-beneficiary",
			Usage: "send confirmation from the new beneficiary address",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), false, true)
		if err != nil {
			return err
		}

		mi, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		if mi.PendingBeneficiaryTerm == nil {
			return fmt.Errorf("no pending beneficiary term found for miner %s", maddr)
		}

		if (cctx.IsSet("existing-beneficiary") && cctx.IsSet("new-beneficiary")) || (!cctx.IsSet("existing-beneficiary") && !cctx.IsSet("new-beneficiary")) {
			return fmt.Errorf("must pass exactly one of --existing-beneficiary or --new-beneficiary")
		}

		var fromAddr address.Address
		if cctx.IsSet("existing-beneficiary") {
			if mi.PendingBeneficiaryTerm.ApprovedByBeneficiary {
				return fmt.Errorf("beneficiary change already approved by current beneficiary")
			}
			fromAddr = mi.Beneficiary
		} else {
			if mi.PendingBeneficiaryTerm.ApprovedByNominee {
				return fmt.Errorf("beneficiary change already approved by new beneficiary")
			}
			fromAddr = mi.PendingBeneficiaryTerm.NewBeneficiary
		}

		fmt.Println("Confirming Pending Beneficiary Term of:")
		fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
		fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
		fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
			return nil
		}

		params := &types.ChangeBeneficiaryParams{
			NewBeneficiary: mi.PendingBeneficiaryTerm.NewBeneficiary,
			NewQuota:       mi.PendingBeneficiaryTerm.NewQuota,
			NewExpiration:  mi.PendingBeneficiaryTerm.NewExpiration,
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		mid, err := api.Messager.PushMessage(ctx, &types.Message{
			From:   fromAddr,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.ChangeBeneficiary,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return fmt.Errorf("push message: %w", err)
		}
		fmt.Println("Propose Message ID:", mid)

		// wait for it to get mined into a block
		wait, err := api.Messager.WaitMessage(ctx, mid, policy.InteractivePoRepConfidence)
		if err != nil {
			return fmt.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("propose beneficiary change failed")
		}

		updatedMinerInfo, err := api.Chain.StateMinerInfo(ctx, maddr, wait.TipSetKey)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == mi.PendingBeneficiaryTerm.NewBeneficiary {
			fmt.Println("Beneficiary address successfully changed")
		} else {
			fmt.Println("Beneficiary address change awaiting additional confirmations")
		}
		return nil
	},
}
