package internal

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/docker/go-units"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	power5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/power"
	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/pkg/specactors"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/messager"
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
			} else {
				return err
			}
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
			fmt.Printf("\t%s\n", addr)
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
		},
		&cli.StringFlag{
			Name: "owner",
		},
		&cli.StringFlag{
			Name: "worker",
		},
		&cli.StringFlag{
			Name:     "sector-size",
			Required: true,
		},
		&cli.StringFlag{
			Name: "peer",
		},
		&cli.StringSliceFlag{
			Name: "multiaddr",
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

		sealProof, err := miner.SealProofTypeFromSectorSize(abi.SectorSize(ssize), constants.NewestNetworkVersion)
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
			id, err := peer.IDFromString(s)
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

		params, err := specactors.SerializeParams(&power5.CreateMinerParams{
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

			To:     builtin5.StoragePowerActorAddr,
			Method: builtin5.MethodsPower.CreateMiner,
			Params: params,

			Value: big.Zero(),
		}

		mblk, err := msg.ToStorageBlock()
		if err != nil {
			return fmt.Errorf("failed to generate msg node: %w", err)
		}

		mid := mblk.Cid().String()
		rmid, err := api.Messager.PushMessageWithId(gctx, mid, msg, &messager.MsgMeta{})
		if err != nil {
			return RPCCallError("PushMessageWithId", err)
		}

		if rmid != mid {
			Log.Warnf("mcid not equal to recv id: %s != %s", mid, rmid)
		}

		mlog = mlog.With("mid", mid)
		var mret *messager.Message
	WAIT_RET:
		for {
			mlog.Info("wait for message receipt")
			time.Sleep(30 * time.Second)

			ret, err := api.Messager.GetMessageByUid(gctx, rmid)
			if err != nil {
				mlog.Warnf("GetMessageByUid: %s", err)
				continue
			}

			switch ret.State {
			case messager.MessageState.OnChainMsg:
				mret = ret
				break WAIT_RET

			case messager.MessageState.NoWalletMsg:
				mlog.Warnf("no wallet available for the sender %s, please check", from)

			default:
				mlog.Infof("msg state: %s", messager.MsgStateToString(ret.State))
			}
		}

		mlog = mlog.With("smcid", mret.SignedCid.String(), "height", mret.Height)

		mlog.Infow("message landed on chain")
		if mret.Receipt.ExitCode != 0 {
			mlog.Warnf("message exec failed: %s(%d)", mret.Receipt.ExitCode, mret.Receipt.ExitCode)
			return nil
		}

		var retval power5.CreateMinerReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(mret.Receipt.ReturnValue)); err != nil {
			return fmt.Errorf("unmarshal message return: %w", err)
		}

		mlog.Infof("miner actor: %s (%s)", retval.IDAddress, retval.RobustAddress)
		return nil
	},
}
