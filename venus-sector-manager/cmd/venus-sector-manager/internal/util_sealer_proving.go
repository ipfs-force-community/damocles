package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	miner8 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/hako/durafmt"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	chain2 "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
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
		utilSealerProvingSectorInfoCmd,
		utilSealerProvingWinningVanillaCmd,
		utilSealerProvingCompactPartitionsCmd,
		utilSealerProvingRecoverFaultsCmd,
	},
}

func EpochTime(curr, e abi.ChainEpoch, blockDelay uint64) string {
	if curr > e {
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(blockDelay)*int64(curr-e))).LimitFirstN(2))
	}

	if curr < e {
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(blockDelay)*int64(e-curr))).LimitFirstN(2))
	}

	return fmt.Sprintf("%d (now)", e)
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

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		head, err := api.Chain.ChainHead(actx)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
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
			return fmt.Errorf("getting miner info: %w", err)
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
			return fmt.Errorf("walking miner deadlines and partitions: %w", err)
		}

		var faultPerc float64
		if proving > 0 {
			faultPerc = float64(faults * 100 / proving)
		}

		blockDelaySecs := policy.NetParams.BlockDelaySecs

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

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
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
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "all",
			Usage:   "Count all sectors (only live sectors are counted by default)",
			Aliases: []string{"a"},
		},
	},
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

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		deadlines, err := api.Chain.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting deadlines: %w", err)
		}

		di, err := api.Chain.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting deadlines: %w", err)
		}

		fmt.Printf("Sealer: %s\n", color.BlueString("%s", maddr))

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\topen\tpartitions\tsectors (faults)\tproven partitions")

		for dlIdx, deadline := range deadlines {
			partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
			}

			provenPartitions, err := deadline.PostSubmissions.Count()
			if err != nil {
				return err
			}

			sectors := uint64(0)
			faults := uint64(0)
			var partitionCount int

			for _, partition := range partitions {
				if !cctx.Bool("all") {
					sc, err := partition.LiveSectors.Count()
					if err != nil {
						return err
					}

					if sc > 0 {
						partitionCount++
					}

					sectors += sc
				} else {
					sc, err := partition.AllSectors.Count()
					if err != nil {
						return err
					}

					partitionCount++
					sectors += sc
				}

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
			gapHeight := uint64(30*60) / policy.NetParams.BlockDelaySecs * gapIdx
			open := HeightToTime(head, di.Open+abi.ChainEpoch(gapHeight), policy.NetParams.BlockDelaySecs)

			_, _ = fmt.Fprintf(tw, "%d\t%s\t%d\t%d (%d)\t%d%s\n", dlIdx, open, partitionCount, sectors, faults, provenPartitions, cur)
		}

		return tw.Flush()
	},
}

var utilSealerProvingDeadlineInfoCmd = &cli.Command{
	Name:  "deadline",
	Usage: "View the current proving period deadline information by its index ",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "sector-nums",
			Aliases: []string{"n"},
			Usage:   "Print sector/fault numbers belonging to this deadline",
		},
		&cli.BoolFlag{
			Name:    "bitfield",
			Aliases: []string{"b"},
			Usage:   "Print partition bitfield stats",
		},
	},
	ArgsUsage: "<deadlineIdx>",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("must pass deadline index")
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse deadline index: %w", err)
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		head, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}

		deadlines, err := api.Chain.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting deadlines: %w", err)
		}

		di, err := api.Chain.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting deadlines: %w", err)
		}

		partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, dlIdx, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
		}

		provenPartitions, err := deadlines[dlIdx].PostSubmissions.Count()
		if err != nil {
			return err
		}

		gapIdx := dlIdx - di.Index
		// 30 minutes a deadline
		gapHeight := uint64(30*60) / policy.NetParams.BlockDelaySecs * gapIdx

		fmt.Printf("Deadline Index:           %d\n", dlIdx)
		fmt.Printf("Deadline Open:            %s\n", HeightToTime(head, di.Open+abi.ChainEpoch(gapHeight), policy.NetParams.BlockDelaySecs))
		fmt.Printf("Partitions:               %d\n", len(partitions))
		fmt.Printf("Proven Partitions:        %d\n", provenPartitions)
		fmt.Printf("Current:                  %t\n\n", di.Index == dlIdx)

		for pIdx, partition := range partitions {
			fmt.Printf("Partition Index:          %d\n", pIdx)

			printStats := func(bf bitfield.BitField, name string) error {
				count, err := bf.Count()
				if err != nil {
					return err
				}

				rit, err := bf.RunIterator()
				if err != nil {
					return err
				}

				if cctx.Bool("bitfield") {
					var ones, zeros, oneRuns, zeroRuns, invalid uint64
					for rit.HasNext() {
						r, err := rit.NextRun()
						if err != nil {
							return fmt.Errorf("next run: %w", err)
						}
						if !r.Valid() {
							invalid++
						}
						if r.Val {
							ones += r.Len
							oneRuns++
						} else {
							zeros += r.Len
							zeroRuns++
						}
					}

					var buf bytes.Buffer
					if err := bf.MarshalCBOR(&buf); err != nil {
						return err
					}
					sz := len(buf.Bytes())
					szstr := types.SizeStr(types.NewInt(uint64(sz)))

					fmt.Printf("\t%s Sectors:%s%d (bitfield - runs %d+%d=%d - %d 0s %d 1s - %d inv - %s %dB)\n", name, strings.Repeat(" ", 18-len(name)), count, zeroRuns, oneRuns, zeroRuns+oneRuns, zeros, ones, invalid, szstr, sz)
				} else {
					fmt.Printf("\t%s Sectors:%s%d\n", name, strings.Repeat(" ", 18-len(name)), count)
				}

				if cctx.Bool("sector-nums") {
					nums, err := bf.All(count)
					if err != nil {
						return err
					}
					fmt.Printf("\t%s Sector Numbers:%s%v\n", name, strings.Repeat(" ", 12-len(name)), nums)
				}

				return nil
			}

			if err := printStats(partition.AllSectors, "All"); err != nil {
				return err
			}
			if err := printStats(partition.LiveSectors, "Live"); err != nil {
				return err
			}
			if err := printStats(partition.ActiveSectors, "Active"); err != nil {
				return err
			}
			if err := printStats(partition.FaultySectors, "Faulty"); err != nil {
				return err
			}
			if err := printStats(partition.RecoveringSectors, "Recovering"); err != nil {
				return err
			}
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
		&cli.BoolFlag{
			Name:  "faulty",
			Usage: "only check faulty sectors",
		},
		&cli.BoolFlag{
			Name:  "detail",
			Usage: "show detail",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		dlIdx, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse deadline index: %w", err)
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		_, err = api.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		partitions, err := api.Chain.StateMinerPartitions(ctx, maddr, dlIdx, types.EmptyTSK)
		if err != nil {
			return err
		}

		var filter *bitfield.BitField = nil
		if cctx.Bool("faulty") {
			t := bitfield.New()
			filter = &t
			for _, part := range partitions {
				*filter, err = bitfield.MergeBitFields(*filter, part.FaultySectors)
				if err != nil {
					return fmt.Errorf("merge faulty sectors: %w", err)
				}
			}
		}

		showDetail := cctx.Bool("detail")
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		if showDetail {
			_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsector\tstatus")
		} else {
			_, _ = fmt.Fprintln(tw, "deadline\tpartition\tgood\tbad")
		}

		slow := cctx.Bool("slow")
		for parIdx, par := range partitions {
			sectors := make(map[abi.SectorNumber]struct{})

			sectorInfos, err := api.Chain.StateMinerSectors(ctx, maddr, &par.LiveSectors, types.EmptyTSK)
			if err != nil {
				return err
			}

			var tocheck []builtin.ExtendedSectorInfo
			for _, info := range sectorInfos {
				if filter != nil {
					found, err := filter.IsSet(uint64(info.SectorNumber))
					if err != nil {
						return err
					}
					if !found {
						continue
					}
				}

				sectors[info.SectorNumber] = struct{}{}
				tocheck = append(tocheck, util.SectorOnChainInfoToExtended(info))
			}

			bad, err := api.Sealer.CheckProvable(ctx, abi.ActorID(mid), tocheck, slow)
			if err != nil {
				return err
			}

			if showDetail {
				for s := range sectors {
					if err, exist := bad[s]; exist {
						_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.RedString("bad")+fmt.Sprintf(" (%s)", err))
					} else if !cctx.Bool("only-bad") {
						_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\n", dlIdx, parIdx, s, color.GreenString("good"))
					}
				}
			} else if len(sectors) != 0 {
				_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d\n", dlIdx, parIdx, len(sectors)-len(bad), len(bad))
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

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
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

		substitute := util.SectorOnChainInfoToExtended(sset[0])

		sectorByID := make(map[uint64]builtin.ExtendedSectorInfo, len(sset))
		for _, sector := range sset {
			sectorByID[uint64(sector.SectorNumber)] = util.SectorOnChainInfoToExtended(sector)
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

		err = api.Sealer.SimulateWdPoSt(ctx, maddr, proofSectors, rand)
		if err != nil {
			return err
		}

		fmt.Printf("Simulate sectors %v wdpost start, please retrieve `mock generate window post` from the log to view execution info.\n", proofSectors)

		return nil
	},
}

var utilSealerProvingSectorInfoCmd = &cli.Command{
	Name:      "sector-info",
	Usage:     "Print sector info ",
	ArgsUsage: "<sector number> ...",
	Action: func(cctx *cli.Context) error {
		api, actx, astop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer astop()

		mid, err := ShouldActor(cctx.String("miner"), true)
		if err != nil {
			return err
		}

		mlog := Log.With("miner", mid)

		args := cctx.Args()
		for i := 0; i < args.Len(); i++ {
			argStr := args.Get(i)
			num, err := strconv.ParseUint(args.Get(i), 10, 64)
			if err != nil {
				mlog.Warnf("#%d %s is not a valid sector number", i, argStr)
				continue
			}

			slog := mlog.With("num", num)

			info, err := api.Sealer.ProvingSectorInfo(actx, abi.SectorID{
				Miner:  mid,
				Number: abi.SectorNumber(num),
			})

			if err != nil {
				slog.Warnf("failed to get info: %w", err)
				continue
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "\t")
			if err := enc.Encode(info); err != nil {
				slog.Warnf("failed to output info: %w", err)
				continue
			}
		}

		return nil
	},
}

var utilSealerProvingWinningVanillaCmd = &cli.Command{
	Name: "winning-vanilla",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "sealed-file",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "cache-dir",
			Required: true,
		},
		&cli.StringFlag{
			Name: "output",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, actx, astop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer astop()

		sealedFilePath, err := filepath.Abs(cctx.String("sealed-file"))
		if err != nil {
			return fmt.Errorf("abs path of sealed file: %w", err)
		}

		cacheDirPath, err := filepath.Abs(cctx.String("cache-dir"))
		if err != nil {
			return fmt.Errorf("abs path of cache dir: %w", err)
		}

		sealedFileName := filepath.Base(sealedFilePath)
		sectorID, ok := util.ScanSectorID(sealedFileName)
		if !ok {
			return fmt.Errorf("invalid formatted sector id: %s", sealedFileName)
		}

		maddr, err := address.NewIDAddress(uint64(sectorID.Miner))
		if err != nil {
			return fmt.Errorf("generate miner address: %w", err)
		}

		sectorOnChainInfo, err := api.Chain.StateSectorGetInfo(actx, maddr, sectorID.Number, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("get sector on chain info: %w", err)
		}

		sectorInfo := core.SectorInfo{
			SealProof:    sectorOnChainInfo.SealProof,
			SectorNumber: sectorOnChainInfo.SectorNumber,
			SealedCID:    sectorOnChainInfo.SealedCID,
		}

		slog := Log.With("sector", sealedFileName, "commR", sectorInfo.SealedCID)
		commR, err := util.CID2ReplicaCommitment(sectorInfo.SealedCID)
		if err != nil {
			return fmt.Errorf("parse commR: %w", err)
		}

		slog.Infof("commR: %v", commR)

		randomness := make(abi.PoStRandomness, abi.RandomnessLength)
		challenges, err := prover.Prover.GeneratePoStFallbackSectorChallenges(actx, abi.RegisteredPoStProof_StackedDrgWinning32GiBV1, sectorID.Miner, randomness, []abi.SectorNumber{sectorID.Number})
		if err != nil {
			return fmt.Errorf("generate challenge for sector %s: %w", sealedFileName, err)
		}

		challenge, ok := challenges.Challenges[sectorID.Number]
		if !ok {
			return fmt.Errorf("challenges for %s not found", sealedFileName)
		}

		slog.Infof("%d challenge generated", len(challenge))

		vannilla, err := prover.Prover.GenerateSingleVanillaProof(actx, core.FFIPrivateSectorInfo{
			SectorInfo:       sectorInfo,
			PoStProofType:    abi.RegisteredPoStProof_StackedDrgWinning32GiBV1,
			CacheDirPath:     cacheDirPath,
			SealedSectorPath: sealedFilePath,
		}, challenge)
		if err != nil {
			return fmt.Errorf("generate vannilla proof for %s: %w", sealedFileName, err)
		}

		slog.Infof("vannilla generated with %d bytes", len(vannilla))

		proofs, err := prover.Prover.GenerateWinningPoStWithVanilla(actx, abi.RegisteredPoStProof_StackedDrgWinning32GiBV1, sectorID.Miner, randomness, [][]byte{vannilla})
		if err != nil {
			return fmt.Errorf("generate winning post with vannilla for %s: %w", sealedFileName, err)
		}

		slog.Infof("proof generated with %d bytes", len(proofs[0].ProofBytes))

		verified, err := prover.Verifier.VerifyWinningPoSt(actx, core.WinningPoStVerifyInfo{
			Randomness:        randomness,
			Proofs:            proofs,
			ChallengedSectors: []core.SectorInfo{sectorInfo},
			Prover:            sectorID.Miner,
		})
		if err != nil {
			return fmt.Errorf("verify winning post proof: %w", err)
		}

		if !verified {
			return fmt.Errorf("winning post not verified")
		}

		if output := cctx.String("output"); output != "" {
			if err := os.WriteFile(output, vannilla, 0644); err != nil {
				return fmt.Errorf("write vannilla proof into file: %w", err)
			}

			slog.Info("vanilla proof output written")
		}

		slog.Info("done")

		return nil
	},
}

var utilSealerProvingCompactPartitionsCmd = &cli.Command{
	Name:  "compact-partitions",
	Usage: "Removes dead sectors from partitions and reduces the number of partitions used if possible",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "deadline",
			Usage:    "the deadline to compact the partitions in",
			Required: true,
		},
		&cli.Int64SliceFlag{
			Name:     "partitions",
			Usage:    "list of partitions to compact sectors in",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "miner",
			Usage: "Specify the address of the miner to run this command",
		},
		&cli.StringFlag{
			Name:  "exid",
			Usage: "external identifier of the message, ensure that we could make the message unique, or we could catch up with a previous message",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, actx, astop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer astop()

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return fmt.Errorf("extract miner address: %w", err)
		}

		minfo, err := api.Chain.StateMinerInfo(actx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		deadline := cctx.Uint64("deadline")
		if deadline > miner8.WPoStPeriodDeadlines {
			return fmt.Errorf("deadline %d out of range", deadline)
		}

		parts := cctx.Int64Slice("partitions")
		if len(parts) == 0 {
			return fmt.Errorf("must include at least one partition to compact")
		}

		Log.Info("compacting %d paritions", len(parts))

		partitions := bitfield.New()
		for _, partition := range parts {
			partitions.Set(uint64(partition))
		}

		params := miner8.CompactPartitionsParams{
			Deadline:   deadline,
			Partitions: partitions,
		}

		sp, err := actors.SerializeParams(&params)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		if !cctx.Bool("really-do-it") {
			Log.Warn("Pass --really-do-it to actually execute this action")
			return nil
		}

		msg := &messager.UnsignedMessage{
			From:   minfo.Worker,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.CompactPartitions,
			Value:  big.Zero(),
			Params: sp,
		}

		err = waitMessage(actx, api, msg, cctx.String("exid"), nil, nil)
		if err != nil {
			return fmt.Errorf("wait message: %w", err)
		}

		return nil
	},
}

var utilSealerProvingRecoverFaultsCmd = &cli.Command{
	Name:  "recover-faults",
	Usage: "Recover faults manually",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "deadline",
			Usage:    "fast path to declare fault with a whole deadline",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "miner",
			Usage: "Specify the address of the miner to run this command",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "Specify the address of the address to send message, default miner's worker",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, actx, astop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer astop()

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return fmt.Errorf("extract miner address: %w", err)
		}

		minfo, err := api.Chain.StateMinerInfo(actx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		ddlIndex := cctx.Uint64("deadline")

		partitions, err := api.Chain.StateMinerPartitions(actx, maddr, ddlIndex, types.EmptyTSK)
		if err != nil {
			return err
		}
		params := miner8.DeclareFaultsRecoveredParams{}
		for idx, partition := range partitions {
			s, err := partition.FaultySectors.All(math.MaxUint64)
			if err != nil {
				return err
			}
			sectors := bitfield.New()
			for _, sid := range s {
				sectors.Set(sid)
			}
			params.Recoveries = append(params.Recoveries, miner8.RecoveryDeclaration{
				Deadline:  ddlIndex,
				Partition: uint64(idx),
				Sectors:   sectors,
			})
		}

		sp, err := actors.SerializeParams(&params)
		if err != nil {
			return fmt.Errorf("serializing params: %w", err)
		}

		sendAddr := minfo.Worker
		if s := cctx.String("from"); s != "" {
			from, err := address.NewFromString(s)
			if err != nil {
				return err
			}
			sendAddr = from
		}

		msg := &messager.UnsignedMessage{
			From:   sendAddr,
			To:     maddr,
			Method: stbuiltin.MethodsMiner.DeclareFaultsRecovered,
			Value:  big.Zero(),
			Params: sp,
		}

		err = waitMessage(actx, api, msg, cctx.String("exid"), nil, nil)
		if err != nil {
			return fmt.Errorf("wait message: %w", err)
		}

		return nil
	},
}
