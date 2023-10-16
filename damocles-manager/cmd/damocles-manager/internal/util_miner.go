package internal

import (
	"errors"
	"fmt"

	"github.com/docker/go-units"
	"github.com/filecoin-project/go-address"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/power"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

var utilMinerCmd = &cli.Command{
	Name:  "miner",
	Usage: "Miner-related utilities",
	Subcommands: []*cli.Command{
		utilMinerInfoCmd,
		utilMinerCreateCmd,
	},
}

var utilMinerInfoCmd = &cli.Command{
	Name:      "info",
	Usage:     "Print miner info",
	ArgsUsage: "<miner address>",
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.Args().First(), true, true)
		if err != nil {
			if errors.Is(err, ErrEmptyAddressString) {
				return ShowHelp(cctx, err)
			}

			return err
		}

		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		minfo, err := api.Chain.StateMinerInfo(gctx, maddr, types.EmptyTSK)
		if err != nil {
			return RPCCallError("StateMinerInfo", err)
		}

		fmt.Printf("Miner: %s\n", maddr)
		fmt.Printf("Owner: %s\n", minfo.Owner)
		fmt.Printf("Worker: %s\n", minfo.Worker)
		fmt.Printf("NewWorker: %s\n", minfo.NewWorker)

		fmt.Printf("Controls(%d):\n", len(minfo.ControlAddresses))
		for _, addr := range minfo.ControlAddresses {
			fmt.Printf("\t%s\n", addr)
		}

		if minfo.Beneficiary != address.Undef {
			fmt.Println()
			fmt.Printf("Beneficiary:\t%s\n", minfo.Beneficiary)
			if minfo.Beneficiary != minfo.Owner {
				fmt.Printf("Beneficiary Quota:\t%s\n", minfo.BeneficiaryTerm.Quota)
				fmt.Printf("Beneficiary Used Quota:\t%s\n", minfo.BeneficiaryTerm.UsedQuota)
				fmt.Printf("Beneficiary Expiration:\t%s\n", minfo.BeneficiaryTerm.Expiration)
			}
		}

		if minfo.PendingBeneficiaryTerm != nil {
			fmt.Printf("Pending Beneficiary Term:\n")
			fmt.Printf("New Beneficiary:\t%s\n", minfo.PendingBeneficiaryTerm.NewBeneficiary)
			fmt.Printf("New Quota:\t%s\n", minfo.PendingBeneficiaryTerm.NewQuota)
			fmt.Printf("New Expiration:\t%s\n", minfo.PendingBeneficiaryTerm.NewExpiration)
			fmt.Printf("Approved By Beneficiary:\t%t\n", minfo.PendingBeneficiaryTerm.ApprovedByBeneficiary)
			fmt.Printf("Approved By Nominee:\t%t\n", minfo.PendingBeneficiaryTerm.ApprovedByNominee)
		}
		fmt.Println()
		fmt.Printf("WokerChangeEpoch: %d\n", minfo.WorkerChangeEpoch)
		if minfo.PeerId != nil {
			fmt.Printf("PeerID: %s\n", *minfo.PeerId)
		}

		fmt.Printf("Multiaddrs(%d):\n", len(minfo.Multiaddrs))
		for _, addr := range minfo.Multiaddrs {
			maddr, err := ma.NewMultiaddrBytes(addr)
			if err != nil {
				fmt.Printf("\tInvalid Multiaddrs: %s\n", err)
			} else {
				fmt.Printf("\t%s\n", maddr)
			}
		}

		fmt.Printf("SectorSize: %s\n", minfo.SectorSize)
		fmt.Printf("WindowPoStPartitionSectors: %d\n", minfo.WindowPoStPartitionSectors)

		return nil
	},
}

var utilMinerCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "Create a new miner_id by sending message",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "from",
			Required: true,
			Usage:    "Wallet address used for sending the `CreateMiner` message",
		},
		&cli.StringFlag{
			Name:  "owner",
			Usage: "Actor address used as the `Owner` field. Will use the AccountActor ID of `from` if not provided",
		},
		&cli.StringFlag{
			Name:  "worker",
			Usage: "Actor address used as the `Worker` field. Will use the AccountActor ID of `from` if not provided",
		},
		&cli.StringFlag{
			Name:     "sector-size",
			Required: true,
			Usage:    "Sector size of the miner, 512MiB, 32GiB, 64GiB, etc",
		},
		&cli.StringFlag{
			Name:  "peer",
			Usage: "P2P peer id of the miner",
		},
		&cli.StringSliceFlag{
			Name:  "multiaddr",
			Usage: "P2P peer address of the miner",
		},
		&cli.StringFlag{
			Name:  "exid",
			Usage: "extra identifier to avoid duplicate msg id for pushing into sophon-messager",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("build apis: %w", err)
		}

		defer stop()

		sizeStr := cctx.String("sector-size")
		ssize, err := units.RAMInBytes(sizeStr)
		if err != nil {
			return fmt.Errorf("failed to parse sector size: %w", err)
		}

		ts, err := api.Chain.ChainHead(gctx)
		if err != nil {
			return fmt.Errorf("get chain head: %w", err)
		}

		tsk := ts.Key()

		fromStr := cctx.String("from")
		from, err := ShouldAddress(fromStr, true, false)
		if err != nil {
			return fmt.Errorf("parse from addr %s: %w", fromStr, err)
		}

		actor, err := api.Chain.StateLookupID(gctx, from, tsk)
		if err != nil {
			return fmt.Errorf("lookup actor address: %w", err)
		}

		mlog := Log.With("size", sizeStr, "from", fromStr)
		mlog.Debug("constructing message")

		owner := actor
		if s := cctx.String("owner"); s != "" {
			addr, err := ShouldAddress(s, false, false)
			if err != nil {
				return fmt.Errorf("parse owner addr %s: %w", s, err)
			}

			owner = addr
			mlog = mlog.With("owner", owner.String())
		}

		worker := owner
		if s := cctx.String("worker"); s != "" {
			addr, err := ShouldAddress(s, false, false)
			if err != nil {
				return fmt.Errorf("parse worker addr %s: %w", s, err)
			}

			worker = addr
			mlog = mlog.With("worker", worker.String())
		}

		var pid abi.PeerID
		if s := cctx.String("peer"); s != "" {
			id, err := peer.Decode(s)
			if err != nil {
				return fmt.Errorf("parse peer id %s: %w", s, err)
			}

			pid = abi.PeerID(id)
		}

		var multiaddrs []abi.Multiaddrs
		for _, one := range cctx.StringSlice("multiaddr") {
			maddr, err := ma.NewMultiaddr(one)
			if err != nil {
				return fmt.Errorf("parse multiaddr %s: %w", one, err)
			}

			maddrNop2p, strip := ma.SplitFunc(maddr, func(c ma.Component) bool {
				return c.Protocol().Code == ma.P_P2P
			})

			if strip != nil {
				fmt.Println("Stripping peerid ", strip, " from ", maddr)
			}

			multiaddrs = append(multiaddrs, maddrNop2p.Bytes())
		}

		nv, err := api.Chain.StateNetworkVersion(gctx, tsk)
		if err != nil {
			return fmt.Errorf("get network version: %w", err)
		}

		postProof, err := miner.WindowPoStProofTypeFromSectorSize(abi.SectorSize(ssize), nv)
		if err != nil {
			return fmt.Errorf("invalid sector size %d: %w", ssize, err)
		}

		params, err := actors.SerializeParams(&core.CreateMinerParams{
			Owner:               owner,
			Worker:              worker,
			WindowPoStProofType: postProof,
			Peer:                pid,
			Multiaddrs:          multiaddrs,
		})

		if err != nil {
			return fmt.Errorf("serialize params: %w", err)
		}

		msg := &messager.UnsignedMessage{
			From: from,

			To:     power.Address,
			Method: power.Methods.CreateMiner,
			Params: params,

			Value: big.Zero(),
		}

		var retval core.CreateMinerReturn
		err = waitMessage(gctx, api, msg, cctx.String("exid"), mlog, &retval)
		if err != nil {
			return err
		}

		fmt.Printf("miner actor: %s (%s)\n", retval.IDAddress, retval.RobustAddress)
		return nil
	},
}
