package internal

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/hako/durafmt"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	chain2 "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
)

var utilSealerProvingCmd = &cli.Command{
	Name:  "proving",
	Usage: "View proving information",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
	},
	Subcommands: []*cli.Command{
		utilSealerProvingInfoCmd,
		utilSealerProvingFaultsCmd,
		utilSealerProvingDeadlinesCmd,
		utilSealerProvingDeadlineInfoCmd,
		utilSealerProvingCheckProvableCmd,
		utilSealerProvingSimulateWdPoStCmd,
	},
}

func getMinerActorAddress(strMaddr string) (maddr address.Address, err error) {
	if strMaddr != "" {
		maddr, err = address.NewFromString(strMaddr)
		if err != nil {
			return maddr, err
		}
		return
	}

	return address.Undef, fmt.Errorf("`miner` param not set")
}

func EpochTime(curr, e abi.ChainEpoch, blockDelay uint64) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(blockDelay)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(blockDelay)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

func HeightToTime(ts *types.TipSet, openHeight abi.ChainEpoch, blockDelay uint64) string {
	if ts.Len() == 0 {
		return ""
	}
	firstBlkTime := ts.Blocks()[0].Timestamp - uint64(ts.Height())*blockDelay
	return time.Unix(int64(firstBlkTime+blockDelay*uint64(openHeight)), 0).Format("15:04:05")
}

var utilSealerProvingInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "View current state information",
	Action: func(cctx *cli.Context) error {
		api, actx, astop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer astop()

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		head, err := api.Chain.ChainHead(actx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		mact, err := api.Chain.StateGetActor(actx, maddr, head.Key())
		if err != nil {
			return err
		}

		stor := chain.ActorStore(actx, chain2.NewAPIBlockstore(api.Chain))

		mas, err := miner.Load(stor, mact)
		if err != nil {
			return err
		}

		cd, err := api.Chain.StateMinerProvingDeadline(actx, maddr, head.Key())
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		color.NoColor = !cctx.Bool("color")

		fmt.Printf("Sealer: %s\n", color.BlueString("%s", maddr))

		proving := uint64(0)
		faults := uint64(0)
		recovering := uint64(0)
		curDeadlineSectors := uint64(0)

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				if bf, err := part.LiveSectors(); err != nil {
					return err
				} else if count, err := bf.Count(); err != nil {
					return err
				} else {
					proving += count
					if dlIdx == cd.Index {
						curDeadlineSectors += count
					}
				}

				if bf, err := part.FaultySectors(); err != nil {
					return err
				} else if count, err := bf.Count(); err != nil {
					return err
				} else {
					faults += count
				}

				if bf, err := part.RecoveringSectors(); err != nil {
					return err
				} else if count, err := bf.Count(); err != nil {
					return err
				} else {
					recovering += count
				}

				return nil
			})
		}); err != nil {
			return xerrors.Errorf("walking miner deadlines and partitions: %w", err)
		}

		var faultPerc float64
		if proving > 0 {
			faultPerc = float64(faults * 100 / proving)
		}

		blockDelaySecs := policy.NetParams.Network.BlockDelay

		fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)

		fmt.Printf("Proving Period Boundary: %d\n", cd.PeriodStart%cd.WPoStProvingPeriod)
		fmt.Printf("Proving Period Start:    %s\n", EpochTime(cd.CurrentEpoch, cd.PeriodStart, blockDelaySecs))
		fmt.Printf("Next Period Start:       %s\n\n", EpochTime(cd.CurrentEpoch, cd.PeriodStart+cd.WPoStProvingPeriod, blockDelaySecs))

		fmt.Printf("Faults:      %d (%.2f%%)\n", faults, faultPerc)
		fmt.Printf("Recovering:  %d\n", recovering)

		fmt.Printf("Deadline Index:       %d\n", cd.Index)
		fmt.Printf("Deadline Sectors:     %d\n", curDeadlineSectors)
		fmt.Printf("Deadline Open:        %s\n", EpochTime(cd.CurrentEpoch, cd.Open, blockDelaySecs))
		fmt.Printf("Deadline Close:       %s\n", EpochTime(cd.CurrentEpoch, cd.Close, blockDelaySecs))
		fmt.Printf("Deadline Challenge:   %s\n", EpochTime(cd.CurrentEpoch, cd.Challenge, blockDelaySecs))
		fmt.Printf("Deadline FaultCutoff: %s\n", EpochTime(cd.CurrentEpoch, cd.FaultCutoff, blockDelaySecs))
		return nil
	},
}

var utilSealerProvingFaultsCmd = &cli.Command{
	Name:  "faults",
	Usage: "View the currently known proving faulty sectors information",
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		stor := chain.ActorStore(ctx, chain2.NewAPIBlockstore(api.Chain))

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		mact, err := api.Chain.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		mas, err := miner.Load(stor, mact)
		if err != nil {
			return err
		}

		fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

		ts, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}
		curHeight := ts.Height()

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsectors\texpiration(days)")
		err = mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				faults, err := part.FaultySectors()
				if err != nil {
					return err
				}
				return faults.ForEach(func(num uint64) error {
					se, err := api.Chain.StateSectorExpiration(ctx, maddr, abi.SectorNumber(num), types.EmptyTSK)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprintf(tw, "  %d\t%d\t%d\t%v\n", dlIdx, partIdx, num, float64((se.Early-curHeight)*builtin.EpochDurationSeconds)/60/60/24)
					return nil
				})
			})
		})
		if err != nil {
			return err
		}
		return tw.Flush()
	},
}

var utilSealerProvingDeadlinesCmd = &cli.Command{
	Name:  "deadlines",
	Usage: "View the current proving period deadlines information",
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		deadlines, err := api.Chain.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadlines: %w", err)
		}

		di, err := api.Chain.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadlines: %w", err)
		}

		fmt.Printf("Sealer: %s\n", color.BlueString("%s", maddr))

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\topen\tpartitions\tsectors (faults)\tproven partitions")

		for dlIdx, deadline := range deadlines {
			partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
			}

			provenPartitions, err := deadline.PostSubmissions.Count()
			if err != nil {
				return err
			}

			sectors := uint64(0)
			faults := uint64(0)

			for _, partition := range partitions {
				sc, err := partition.AllSectors.Count()
				if err != nil {
					return err
				}

				sectors += sc

				fc, err := partition.FaultySectors.Count()
				if err != nil {
					return err
				}

				faults += fc
			}

			var cur string
			if di.Index == uint64(dlIdx) {
				cur += "\t(current)"
			}

			gapIdx := uint64(dlIdx) - di.Index
			// 30 minutes a deadline
			gapHeight := uint64(30*60) / policy.NetParams.Network.BlockDelay * gapIdx
			open := HeightToTime(head, di.Open+abi.ChainEpoch(gapHeight), policy.NetParams.Network.BlockDelay)

			_, _ = fmt.Fprintf(tw, "%d\t%s\t%d\t%d (%d)\t%d%s\n", dlIdx, open, len(partitions), sectors, faults, provenPartitions, cur)
		}

		return tw.Flush()
	},
}

var utilSealerProvingDeadlineInfoCmd = &cli.Command{
	Name:      "deadline",
	Usage:     "View the current proving period deadline information by its index ",
	ArgsUsage: "<deadlineIdx>",
	Action: func(cctx *cli.Context) error {

		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass deadline index")
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse deadline index: %w", err)
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		deadlines, err := api.Chain.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadlines: %w", err)
		}

		di, err := api.Chain.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadlines: %w", err)
		}

		partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, dlIdx, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
		}

		provenPartitions, err := deadlines[dlIdx].PostSubmissions.Count()
		if err != nil {
			return err
		}

		gapIdx := dlIdx - di.Index
		// 30 minutes a deadline
		gapHeight := uint64(30*60) / policy.NetParams.Network.BlockDelay * gapIdx

		fmt.Printf("Deadline Index:           %d\n", dlIdx)
		fmt.Printf("Deadline Open:            %s\n", HeightToTime(head, di.Open+abi.ChainEpoch(gapHeight), policy.NetParams.Network.BlockDelay))
		fmt.Printf("Partitions:               %d\n", len(partitions))
		fmt.Printf("Proven Partitions:        %d\n", provenPartitions)
		fmt.Printf("Current:                  %t\n\n", di.Index == dlIdx)

		for pIdx, partition := range partitions {
			sectorCount, err := partition.AllSectors.Count()
			if err != nil {
				return err
			}

			sectorNumbers, err := partition.AllSectors.All(sectorCount)
			if err != nil {
				return err
			}

			faultsCount, err := partition.FaultySectors.Count()
			if err != nil {
				return err
			}

			fn, err := partition.FaultySectors.All(faultsCount)
			if err != nil {
				return err
			}

			fmt.Printf("Partition Index:          %d\n", pIdx)
			fmt.Printf("Sectors:                  %d\n", sectorCount)
			fmt.Printf("Sector Numbers:           %v\n", sectorNumbers)
			fmt.Printf("Faults:                   %d\n", faultsCount)
			fmt.Printf("Faulty Sectors:           %d\n", fn)
		}
		return nil
	},
}

var utilSealerProvingCheckProvableCmd = &cli.Command{
	Name:      "check",
	Usage:     "Check sectors provable",
	ArgsUsage: "<deadlineIdx>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "only-bad",
			Usage: "print only bad sectors",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "slow",
			Usage: "run slower checks",
		},
		&cli.StringFlag{
			Name:  "storage-id",
			Usage: "filter sectors by storage path (path id)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass deadline index")
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse deadline index: %w", err)
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		info, err := api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, dlIdx, types.EmptyTSK)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsector\tstatus")

		sapi, sctx, stop, err := extractSealerClient(cctx)
		if err != nil {
			return err
		}
		defer stop()

		var filter map[abi.SectorID]struct{}
		for parIdx, par := range partitions {
			sectors := make(map[abi.SectorNumber]struct{})

			sectorInfos, err := api.Chain.StateMinerSectors(ctx, maddr, &par.LiveSectors, types.EmptyTSK)
			if err != nil {
				return err
			}

			var tocheck []storage.SectorRef
			for _, info := range sectorInfos {
				si := abi.SectorID{
					Miner:  abi.ActorID(mid),
					Number: info.SectorNumber,
				}

				if filter != nil {
					if _, found := filter[si]; !found {
						continue
					}
				}

				sectors[info.SectorNumber] = struct{}{}
				tocheck = append(tocheck, storage.SectorRef{
					ProofType: info.SealProof,
					ID:        si,
				})
			}

			bad, err := sapi.CheckProvable(sctx, info.WindowPoStProofType, tocheck, cctx.Bool("slow"))
			if err != nil {
				return err
			}

			for s := range sectors {
				if err, exist := bad[s]; exist {
					_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.RedString("bad")+fmt.Sprintf(" (%s)", err))
				} else if !cctx.Bool("only-bad") {
					_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.GreenString("good"))
				}
			}
		}

		return tw.Flush()
	},
}

var utilSealerProvingSimulateWdPoStCmd = &cli.Command{
	Name:  "simulate-wdpost",
	Usage: "Do not execute during normal wdPoSt operation, so as not to occupy sectors or gpu",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "ddl-idx",
			Required: true,
			Usage:    "specify the ddl which need mock",
		},
		&cli.Uint64Flag{
			Name:     "partition-idx",
			Required: true,
			Usage:    "specify the partition which need mock",
		},
		&cli.BoolFlag{
			Name:  "include-faulty",
			Value: false,
			Usage: "whether include faulty sectors or not",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		ts, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return fmt.Errorf("get chain head failed: %w", err)
		}

		maddr, err := getMinerActorAddress(cctx.String("miner"))
		if err != nil {
			return err
		}

		partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, cctx.Uint64("ddl-idx"), ts.Key())
		if err != nil {
			return fmt.Errorf("get parttion info failed: %w", err)
		}
		pidx := cctx.Uint64("partition-idx")
		if uint64(len(partitions)) <= pidx {
			return fmt.Errorf("partition-idx is range out of partitions array: %d <= %d", len(partitions), pidx)
		}

		toProve := partitions[pidx].LiveSectors
		if !cctx.Bool("include-faulty") {
			if toProve, err = bitfield.SubtractBitField(partitions[pidx].LiveSectors, partitions[pidx].FaultySectors); err != nil {
				return err
			}
		}

		if toProve, err = bitfield.MergeBitFields(toProve, partitions[pidx].RecoveringSectors); err != nil {
			return err
		}

		sset, err := api.Chain.StateMinerSectors(ctx, maddr, &toProve, ts.Key())
		if err != nil {
			return fmt.Errorf("get miner sectors failed: %w", err)
		}

		if len(sset) == 0 {
			return fmt.Errorf("no lived sector in that partition")
		}

		substitute := builtin.ExtendedSectorInfo{
			SectorNumber: sset[0].SectorNumber,
			SealedCID:    sset[0].SealedCID,
			SealProof:    sset[0].SealProof,
		}

		sectorByID := make(map[uint64]builtin.ExtendedSectorInfo, len(sset))
		for _, sector := range sset {
			sectorByID[uint64(sector.SectorNumber)] = builtin.ExtendedSectorInfo{
				SectorNumber: sector.SectorNumber,
				SealedCID:    sector.SealedCID,
				SealProof:    sector.SealProof,
			}
		}

		proofSectors := make([]builtin.ExtendedSectorInfo, 0, len(sset))
		if err := partitions[pidx].AllSectors.ForEach(func(sectorNo uint64) error {
			if info, found := sectorByID[sectorNo]; found {
				proofSectors = append(proofSectors, info)
			} else {
				proofSectors = append(proofSectors, substitute)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("iterating partition sector bitmap: %w", err)
		}

		rand := abi.PoStRandomness{}
		for i := 0; i < 32; i++ {
			rand = append(rand, 0)
		}

		sapi, sctx, stop, err := extractSealerClient(cctx)
		if err != nil {
			return err
		}
		defer stop()

		err = sapi.SimulateWdPoSt(sctx, maddr, proofSectors, rand)
		if err != nil {
			return err
		}

		fmt.Printf("Simulate sectors %v wdpost start, please retrieve `mock generate window post` from the log to view execution info.\n", proofSectors)

		return nil
	},
}
