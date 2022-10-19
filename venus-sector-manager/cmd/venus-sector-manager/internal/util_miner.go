package internal

import (
	"errors"
	"fmt"

	"github.com/docker/go-units"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/power"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

var utilMinerCmd = &cli.Command{
	Name: "miner",
	Subcommands: []*cli.Command{
		utilMinerInfoCmd,
		utilMinerCreateCmd,
	},
}

var utilMinerInfoCmd = &cli.Command{
	Name:      "info",
	ArgsUsage: "[miner address]",
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
	Name: "create",
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
			Usage: "extra identifier to avoid duplicate msg id for pushing into venus-messager",
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

		sealProof, err := miner.SealProofTypeFromSectorSize(abi.SectorSize(ssize), constants.TestNetworkVersion)
		if err != nil {
			return fmt.Errorf("invalid sector size %d: %w", ssize, err)
		}

		postProof, err := sealProof.RegisteredWindowPoStProof()
		if err != nil {
			return fmt.Errorf("invalid seal proof type %d: %w", sealProof, err)
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

		mlog := Log.With("size", sizeStr, "from", fromStr, "actor", actor.String())
		mlog.Info("constructing message")

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

		mlog.Infof("miner actor: %s (%s)", retval.IDAddress, retval.RobustAddress)
		return nil
	},
}
