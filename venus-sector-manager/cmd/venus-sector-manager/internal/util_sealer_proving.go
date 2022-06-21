package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/hako/durafmt"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
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
		utilSealerProvingSectorInfoCmd,
		utilSealerProvingWinningVanillaCmd,
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

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "deadline\tpartition\tsector\tstatus")

		var filter map[abi.SectorID]struct{}
		for parIdx, par := range partitions {
			sectors := make(map[abi.SectorNumber]struct{})

			sectorInfos, err := api.Chain.StateMinerSectors(ctx, maddr, &par.LiveSectors, types.EmptyTSK)
			if err != nil {
				return err
			}

			var tocheck []builtin.ExtendedSectorInfo
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
				tocheck = append(tocheck, util.SectorOnChainInfoToExtended(info))
			}

			bad, err := api.Sealer.CheckProvable(ctx, abi.ActorID(mid), tocheck, cctx.Bool("slow"))
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

		sectorInfo, err := api.Chain.StateSectorGetInfo(actx, maddr, sectorID.Number, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("get sector on chain info: %w", err)
		}

		slog := Log.With("sector", sealedFileName)

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
			SectorInfo: core.SectorInfo{
				SealProof:    sectorInfo.SealProof,
				SectorNumber: sectorInfo.SectorNumber,
				SealedCID:    sectorInfo.SealedCID,
			},
			PoStProofType:    abi.RegisteredPoStProof_StackedDrgWinning32GiBV1,
			CacheDirPath:     cacheDirPath,
			SealedSectorPath: sealedFilePath,
		}, challenge)
		if err != nil {
			return fmt.Errorf("generate vannilla proof for %s: %w", sealedFileName, err)
		}

		slog.Infof("vannilla generated with %d bytes", len(vannilla))

		proof, err := prover.Prover.GenerateWinningPoStWithVanilla(actx, abi.RegisteredPoStProof_StackedDrgWinning32GiBV1, sectorID.Miner, randomness, [][]byte{vannilla})
		if err != nil {
			return fmt.Errorf("generate winning post with vannilla for %s: %w", sealedFileName, err)
		}

		slog.Infof("proof generated with %d bytes", len(proof))

		verified, err := prover.Verifier.VerifyWinningPoSt(actx, core.WinningPoStVerifyInfo{})
		if err != nil {
			return fmt.Errorf("verify winning post proof: %w", err)
		}

		if !verified {
			return fmt.Errorf("winning post not verified")
		}

		if output := cctx.String("output"); output != "" {
			if err := ioutil.WriteFile(output, vannilla, 0644); err != nil {
				return fmt.Errorf("write vannilla proof into file: %w", err)
			}

			slog.Info("vanilla proof output written")
		}

		slog.Info("done")

		return nil
	},
}
