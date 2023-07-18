package internal

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/venus/pkg/util/fr32"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/verifreg"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	levelds "github.com/ipfs/go-ds-leveldb"
	u "github.com/ipfs/go-ipfs-util"
	cbor "github.com/ipfs/go-ipld-cbor"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/adt"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	chain2 "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/lotusminer"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

var flagListOffline = &cli.BoolFlag{
	Name:  "offline",
	Usage: "show offline data in listing",
	Value: false,
}

var flagListEnableSealing = &cli.BoolFlag{
	Name:  "sealing",
	Usage: "enable sealing jobs in listing",
	Value: true,
}

var flagListEnableSnapup = &cli.BoolFlag{
	Name:  "snapup",
	Usage: "enable snapup jobs in listing",
	Value: false,
}

var flagListEnableRebuild = &cli.BoolFlag{
	Name:  "rebuild",
	Usage: "enable rebuild jobs in listing",
	Value: false,
}

var flagListEnableUnseal = &cli.BoolFlag{
	Name:  "unseal",
	Usage: "enable unseal jobs in listing",
	Value: false,
}

var utilSealerSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Commands for interacting with sectors",
	Subcommands: []*cli.Command{
		utilSealerSectorsAbortCmd,
		utilSealerSectorsListCmd,
		utilSealerSectorsRestoreCmd,
		utilSealerSectorsCheckExpireCmd,
		utilSealerSectorsExpiredCmd,
		utilSealerSectorsExtendCmd,
		utilSealerSectorsTerminateCmd,
		utilSealerSectorsRemoveCmd,
		utilSealerSectorsFinalizeCmd,
		utilSealerSectorsStateCmd,
		utilSealerSectorsFindDealCmd,
		utilSealerSectorsResendPreCommitCmd,
		utilSealerSectorsResendProveCommitCmd,
		utilSealerSectorsImportCmd,
		utilSealerSectorsRebuildCmd,
		utilSealerSectorsExportCmd,
		utilSealerSectorsUnsealCmd,
	},
}

func extractListWorkerState(cctx *cli.Context) core.SectorWorkerState {
	if cctx.Bool(flagListOffline.Name) {
		return core.WorkerOffline
	}

	return core.WorkerOnline
}

var utilSealerSectorsAbortCmd = &cli.Command{
	Name:      "abort",
	Usage:     "Abort specified online sector job",
	ArgsUsage: "<miner actor> <sector number>",
	Action: func(cctx *cli.Context) error {
		return fmt.Errorf("this command is not available in the current version, please use the `damocles-worker worker -c <config file path> resume --state Aborted --index <index>` or `damocles-manager util worker resume <worker instance name or address> <thread index> Aborted` commands instead.\n See: https://github.com/ipfs-force-community/damocles/blob/main/docs/en/11.task-status-flow.md#1-for-a-sector-sealing-task-that-has-been-paused-due-to-an-error-and-cannot-be-resumed-such-as-the-ticket-has-expired-you-can-use")
		// if count := cctx.Args().Len(); count < 2 {
		// 	return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		// }

		// miner, err := ShouldActor(cctx.Args().Get(0), true)
		// if err != nil {
		// 	return fmt.Errorf("invalid miner actor id: %w", err)
		// }

		// sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		// if err != nil {
		// 	return fmt.Errorf("invalid sector number: %w", err)
		// }

		// cli, gctx, stop, err := extractAPI(cctx)
		// if err != nil {
		// 	return err
		// }

		// defer stop()

		// _, err = cli.Sealer.ReportAborted(gctx, abi.SectorID{
		// 	Miner:  miner,
		// 	Number: abi.SectorNumber(sectorNum),
		// }, "aborted via CLI")
		// if err != nil {
		// 	return fmt.Errorf("abort sector failed: %w", err)
		// }
	},
}

var utilSealerSectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "Print sector data",
	Flags: []cli.Flag{
		flagListOffline,
		flagListEnableSealing,
		flagListEnableSnapup,
		flagListEnableRebuild,
		&cli.StringFlag{
			Name:  "miner",
			Usage: "show sectors of the given miner only ",
		},
	},
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, extractListWorkerState(cctx), core.SectorWorkerJobAll)
		if err != nil {
			return err
		}

		var minerID *abi.ActorID
		if m := cctx.String("miner"); m != "" {
			mid, err := ShouldActor(m, true)
			if err != nil {
				return fmt.Errorf("invalid miner actor id: %w", err)
			}

			minerID = &mid
		}

		selectors := []struct {
			name    string
			jobType core.SectorWorkerJob
		}{
			{
				name:    flagListEnableSealing.Name,
				jobType: core.SectorWorkerJobSealing,
			},
			{
				name:    flagListEnableSnapup.Name,
				jobType: core.SectorWorkerJobSnapUp,
			},
			{
				name:    flagListEnableRebuild.Name,
				jobType: core.SectorWorkerJobRebuild,
			},
			{
				name:    flagListEnableUnseal.Name,
				jobType: core.SectorWorkerJobUnseal,
			},
		}

		count := 0
		fmt.Fprintln(os.Stdout, "Sectors:")

		for _, state := range states {
			if minerID != nil && state.ID.Miner != *minerID {
				continue
			}

			flag := false
			for _, sel := range selectors {
				selectorMatched := cctx.Bool(sel.name)
				typeMatched := state.MatchWorkerJob(sel.jobType)

				if selectorMatched && typeMatched {
					flag = true
					break
				}
			}

			if !flag {
				continue
			}
			count++

			marks := make([]string, 0, 2)
			if state.Upgraded {
				marks = append(marks, "upgrade")
			}

			if state.NeedRebuild {
				marks = append(marks, "rebuild")
			}

			if state.Unsealing {
				marks = append(marks, "unseal")
			}

			var sectorMark string
			if len(marks) > 0 {
				sectorMark = fmt.Sprintf("(%s)", strings.Join(marks, ", "))
			}

			fmt.Fprintf(os.Stdout, "%s%s:\n", util.FormatSectorID(state.ID), sectorMark)

			if state.LatestState != nil {
				fmt.Fprintf(os.Stdout, "\tWorker: %s @ %s\n", state.LatestState.Worker.Instance, state.LatestState.Worker.Location)
				fmt.Fprintf(os.Stdout, "\tState: %s => %s @%s\n", state.LatestState.StateChange.Prev, state.LatestState.StateChange.Next, state.LatestState.StateChange.Event)
			}

			fmt.Fprintf(os.Stdout, "\tFinalized: %v, Removed: %v\n", state.Finalized, state.Removed)

			fmt.Fprintln(os.Stdout, "")
		}

		fmt.Fprintf(os.Stdout, "Count: %d\n", count)

		return nil
	},
}

var utilSealerSectorsRestoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "Restore a sector state that may already finalized or aborted",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:   "force",
			Hidden: true,
			Value:  false,
		},
	},
	ArgsUsage: "<miner actor id> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.RestoreSector(gctx, abi.SectorID{
			Miner:  miner,
			Number: abi.SectorNumber(sectorNum),
		}, cctx.Bool("force"))
		if err != nil {
			return fmt.Errorf("restore sector failed: %w", err)
		}

		return nil
	},
}

var utilSealerSectorsCheckExpireCmd = &cli.Command{
	Name:  "check-expire",
	Usage: "Inspect expiring sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Int64Flag{
			Name:  "cutoff",
			Usage: "skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days",
			Value: 172800,
		},
	},
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
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
		currEpoch := head.Height()

		nv, err := api.Chain.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		sectors, err := api.Chain.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		n := 0
		for _, s := range sectors {
			if s.Expiration-currEpoch <= abi.ChainEpoch(cctx.Int64("cutoff")) {
				sectors[n] = s
				n++
			}
		}
		sectors = sectors[:n]

		sort.Slice(sectors, func(i, j int) bool {
			if sectors[i].Expiration == sectors[j].Expiration {
				return sectors[i].SectorNumber < sectors[j].SectorNumber
			}
			return sectors[i].Expiration < sectors[j].Expiration
		})

		blockDelaySecs := policy.NetParams.BlockDelaySecs
		fmt.Fprintf(os.Stdout, "Sectors(%d):\n", len(sectors))
		for _, sector := range sectors {
			MaxExpiration := sector.Activation + specpolicy.GetSectorMaxLifetime(sector.SealProof, nv)
			MaxExtendNow := currEpoch + specpolicy.GetMaxSectorExpirationExtension()

			if MaxExtendNow > MaxExpiration {
				MaxExtendNow = MaxExpiration
			}

			fmt.Fprintf(os.Stdout, "\tID: %d\n", sector.SectorNumber)
			fmt.Fprintf(os.Stdout, "\tSealProof: %d\n", sector.SealProof)
			fmt.Fprintf(os.Stdout, "\tInitialPledge: %v\n", types.FIL(sector.InitialPledge).Short())
			fmt.Fprintf(os.Stdout, "\tActivation: %s\n", EpochTime(currEpoch, sector.Activation, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tExpiration: %s\n", EpochTime(currEpoch, sector.Expiration, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tMaxExpiration: %s\n", EpochTime(currEpoch, MaxExpiration, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tMaxExtendNow: %s\n", EpochTime(currEpoch, MaxExtendNow, blockDelaySecs))

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}

var utilSealerSectorsExpiredCmd = &cli.Command{
	Name:  "expired",
	Usage: "Get or cleanup expired sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},

		&cli.BoolFlag{
			Name:  "show-removed",
			Usage: "show removed sectors",
		},
		&cli.BoolFlag{
			Name:  "remove-expired",
			Usage: "remove expired sectors",
		},

		&cli.Int64Flag{
			Name:   "confirm-remove-count",
			Hidden: true,
		},
		&cli.Int64Flag{
			Name:        "expired-epoch",
			Usage:       "epoch at which to check sector expirations",
			DefaultText: "WinningPoSt lookback epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		extAPI, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := extAPI.Chain.ChainHead(ctx)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
		}

		lbEpoch := abi.ChainEpoch(cctx.Int64("expired-epoch"))
		if !cctx.IsSet("expired-epoch") {
			nv, err := extAPI.Chain.StateNetworkVersion(ctx, head.Key())
			if err != nil {
				return fmt.Errorf("getting network version: %w", err)
			}

			lbEpoch = head.Height() - specpolicy.GetWinningPoStSectorSetLookback(nv)
			if lbEpoch < 0 {
				return fmt.Errorf("too early to terminate sectors")
			}
		}

		if cctx.IsSet("confirm-remove-count") && !cctx.IsSet("expired-epoch") {
			return fmt.Errorf("--expired-epoch must be specified with --confirm-remove-count")
		}

		lbts, err := extAPI.Chain.ChainGetTipSetByHeight(ctx, lbEpoch, head.Key())
		if err != nil {
			return fmt.Errorf("getting lookback tipset: %w", err)
		}

		// toCheck is a working bitfield which will only contain terminated sectors
		toCheck := bitfield.New()
		toCheckSectors := make(map[abi.SectorNumber]*core.SectorState)
		{
			sectors, err := extAPI.Sealer.ListSectors(ctx, core.WorkerOffline, core.SectorWorkerJobAll)
			if err != nil {
				return fmt.Errorf("getting sector list: %w", err)
			}

			for _, sector := range sectors {
				toCheck.Set(uint64(sector.ID.Number))
				toCheckSectors[sector.ID.Number] = sector
			}
		}

		mact, err := extAPI.Chain.StateGetActor(ctx, maddr, lbts.Key())
		if err != nil {
			return err
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(chain2.NewAPIBlockstore(extAPI.Chain)))
		mas, err := miner.Load(store, mact)
		if err != nil {
			return err
		}

		alloc, err := mas.GetAllocatedSectors()
		if err != nil {
			return fmt.Errorf("getting allocated sectors: %w", err)
		}

		// only allocated sectors can be expired,
		toCheck, err = bitfield.IntersectBitField(toCheck, *alloc)
		if err != nil {
			return fmt.Errorf("intersecting bitfields: %w", err)
		}

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				live, err := part.LiveSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, live)
				if err != nil {
					return err
				}

				unproven, err := part.UnprovenSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, unproven)

				return err
			})
		}); err != nil {
			return err
		}

		err = mas.ForEachPrecommittedSector(func(pci stminer.SectorPreCommitOnChainInfo) error {
			toCheck.Unset(uint64(pci.Info.SectorNumber))
			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			color.Red("Removing sectors:\n")
		}

		// toCheck now only contains sectors which either failed to precommit or are expired/terminated
		fmt.Printf("SectorID\n")

		var toRemove []abi.SectorNumber
		err = toCheck.ForEach(func(u uint64) error {
			sn := abi.SectorNumber(u)

			if sector, ok := toCheckSectors[sn]; ok {
				if sector.Removed {
					if cctx.IsSet("confirm-remove-count") || !cctx.Bool("show-removed") {
						return nil
					}
				} else { // not removed
					toRemove = append(toRemove, sn)
				}

				fmt.Printf("%d\n", sn)
			}

			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			if !cctx.IsSet("confirm-remove-count") {
				fmt.Println()
				fmt.Println(color.YellowString("All"), color.GreenString("%d", len(toRemove)), color.YellowString("sectors listed above will be removed\n"))
				fmt.Println(color.YellowString("To confirm removal of the above sectors, including\n all related sealed and unsealed data, run:\n"))
				fmt.Println(color.RedString("venus-sealer sectors expired --remove-expired --confirm-remove-count=%d --expired-epoch=%d\n", len(toRemove), lbts.Height()))
				fmt.Println(color.YellowString("WARNING: This operation is irreversible"))
				return nil
			}

			fmt.Println()

			if int64(len(toRemove)) != cctx.Int64("confirm-remove-count") {
				return fmt.Errorf("value of confirm-remove-count doesn't match the number of sectors which can be removed (%d)", len(toRemove))
			}

			actor, _ := address.IDFromAddress(maddr)
			for _, number := range toRemove {
				fmt.Printf("Removing sector\t%s:\t", color.YellowString("%d", number))

				err = extAPI.Sealer.RemoveSector(ctx, abi.SectorID{Miner: abi.ActorID(actor), Number: number})
				if err != nil {
					color.Red("ERROR: %s\n", err.Error())
				} else {
					color.Green("OK\n")
				}
			}
		}

		return nil
	},
}

func getSectorsFromFile(filePath string) ([]abi.SectorNumber, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	sectors := make([]abi.SectorNumber, 0)

	for scanner.Scan() {
		line := scanner.Text()

		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s as sector id: %s", line, err)
		}

		sectors = append(sectors, abi.SectorNumber(id))
	}

	if err = file.Close(); err != nil {
		return nil, err
	}

	return sectors, nil
}

func SectorNumsToBitfield(sectors []abi.SectorNumber) bitfield.BitField {
	var numbers []uint64
	for _, sector := range sectors {
		numbers = append(numbers, uint64(sector))
	}

	return bitfield.NewFromSet(numbers)
}

type PseudoExpirationExtension struct {
	Deadline      uint64
	Partition     uint64
	Sectors       string
	NewExpiration abi.ChainEpoch
}

type PseudoExtendSectorExpirationParams struct {
	Extensions []PseudoExpirationExtension
}

// ArrayToString Example: {1,3,4,5,8,9} -> "1,3-5,8-9"
func ArrayToString(array []uint64) string {
	sort.Slice(array, func(i, j int) bool {
		return array[i] < array[j]
	})

	var sarray []string
	s := ""

	for i, elm := range array {
		if i == 0 {
			s = strconv.FormatUint(elm, 10)
			continue
		}
		if elm == array[i-1] {
			continue // filter out duplicates
		} else if elm == array[i-1]+1 {
			s = strings.Split(s, "-")[0] + "-" + strconv.FormatUint(elm, 10)
		} else {
			sarray = append(sarray, s)
			s = strconv.FormatUint(elm, 10)
		}
	}

	if s != "" {
		sarray = append(sarray, s)
	}

	return strings.Join(sarray, ",")
}

func NewPseudoExtendParams(p *core.ExtendSectorExpiration2Params) (*PseudoExtendSectorExpirationParams, error) {
	res := PseudoExtendSectorExpirationParams{}
	for _, ext := range p.Extensions {
		scount, err := ext.Sectors.Count()
		if err != nil {
			return nil, err
		}

		sectors, err := ext.Sectors.All(scount)
		if err != nil {
			return nil, err
		}

		res.Extensions = append(res.Extensions, PseudoExpirationExtension{
			Deadline:      ext.Deadline,
			Partition:     ext.Partition,
			Sectors:       ArrayToString(sectors),
			NewExpiration: ext.NewExpiration,
		})
	}
	return &res, nil
}

var utilSealerSectorsExtendCmd = &cli.Command{
	Name:      "extend",
	Usage:     "Extend expiring sectors while not exceeding each sector's max life",
	ArgsUsage: "<sectorNumbers...(optional)>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Int64Flag{
			Name:  "from",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour)",
		},
		&cli.Int64Flag{
			Name:  "to",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days)",
		},
		&cli.StringFlag{
			Name:  "sector-file",
			Usage: "provide a file containing one sector number in each line, ignoring above selecting criteria",
		},
		&cli.StringFlag{
			Name:  "exclude",
			Usage: "optionally provide a file containing excluding sectors",
		},
		&cli.Int64Flag{
			Name:  "extension",
			Usage: "try to extend selected sectors by this number of epochs, defaults to 540 days",
			Value: 1555200,
		},
		&cli.Int64Flag{
			Name:  "new-expiration",
			Usage: "try to extend selected sectors to this epoch, ignoring extension",
		},
		&cli.BoolFlag{
			Name:  "only-cc",
			Usage: "only extend CC sectors (useful for making sector ready for snap upgrade)",
		},
		&cli.BoolFlag{
			Name:  "drop-claims",
			Usage: "drop claims for sectors that can be extended, but only by dropping some of their verified power claims",
		},
		&cli.Int64Flag{
			Name:  "tolerance",
			Usage: "don't try to extend sectors by fewer than this number of epochs, defaults to 7 days",
			Value: 20160,
		},
		&cli.StringFlag{
			Name:  "max-fee",
			Usage: "use up to this amount of FIL for one message. pass this flag to avoid message congestion.",
			Value: "0",
		},
		&cli.Int64Flag{
			Name:  "max-sectors",
			Usage: "the maximum number of sectors contained in each message",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag to really extend sectors, otherwise will only print out json representation of parameters",
		},
	},
	Action: func(cctx *cli.Context) error {
		mf, err := types.ParseFIL(cctx.String("max-fee"))
		if err != nil {
			return err
		}
		spec := &messager.MsgMeta{MaxFee: abi.TokenAmount(mf)}

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		fapi, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := fapi.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}
		currEpoch := head.Height()

		nv, err := fapi.Chain.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		activeSet, err := fapi.Chain.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		activeSectorsInfo := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(activeSet))
		for _, info := range activeSet {
			activeSectorsInfo[info.SectorNumber] = info
		}

		mact, err := fapi.Chain.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(chain2.NewAPIBlockstore(fapi.Chain)))
		mas, err := miner.Load(store, mact)
		if err != nil {
			return err
		}

		activeSectorsLocation := make(map[abi.SectorNumber]*miner.SectorLocation, len(activeSet))

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				pas, err := part.ActiveSectors()
				if err != nil {
					return err
				}

				return pas.ForEach(func(i uint64) error {
					activeSectorsLocation[abi.SectorNumber(i)] = &miner.SectorLocation{
						Deadline:  dlIdx,
						Partition: partIdx,
					}
					return nil
				})
			})
		}); err != nil {
			return err
		}

		excludeSet := make(map[abi.SectorNumber]struct{})

		if cctx.IsSet("exclude") {
			excludeSectors, err := getSectorsFromFile(cctx.String("exclude"))
			if err != nil {
				return err
			}

			for _, id := range excludeSectors {
				excludeSet[id] = struct{}{}
			}
		}

		var sectors []abi.SectorNumber
		if cctx.Args().Present() {
			if cctx.IsSet("sector-file") {
				return fmt.Errorf("sector-file specified along with command line params")
			}

			for i, s := range cctx.Args().Slice() {
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return fmt.Errorf("could not parse sector %d: %w", i, err)
				}

				sectors = append(sectors, abi.SectorNumber(id))
			}
		} else if cctx.IsSet("sector-file") {
			sectors, err = getSectorsFromFile(cctx.String("sector-file"))
			if err != nil {
				return err
			}
		} else {
			from := currEpoch + 120
			to := currEpoch + 92160

			if cctx.IsSet("from") {
				from = abi.ChainEpoch(cctx.Int64("from"))
			}

			if cctx.IsSet("to") {
				to = abi.ChainEpoch(cctx.Int64("to"))
			}

			for _, si := range activeSet {
				if si.Expiration >= from && si.Expiration <= to {
					sectors = append(sectors, si.SectorNumber)
				}
			}
		}

		var sis []*miner.SectorOnChainInfo
		onlyCC := cctx.Bool("only-cc")
		for _, id := range sectors {
			if _, exclude := excludeSet[id]; exclude {
				continue
			}

			si, found := activeSectorsInfo[id]
			if !found {
				return fmt.Errorf("sector %d is not active", id)
			}
			if len(si.DealIDs) > 0 && onlyCC {
				continue
			}

			sis = append(sis, si)
		}

		withinTolerance := func(a, b abi.ChainEpoch) bool {
			diff := a - b
			if diff < 0 {
				diff = -diff
			}

			return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
		}

		extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]abi.SectorNumber{}
		for _, si := range sis {
			extension := abi.ChainEpoch(cctx.Int64("extension"))
			newExp := si.Expiration + extension

			if cctx.IsSet("new-expiration") {
				newExp = abi.ChainEpoch(cctx.Int64("new-expiration"))
			}

			maxExtendNow := currEpoch + specpolicy.GetMaxSectorExpirationExtension()
			if newExp > maxExtendNow {
				newExp = maxExtendNow
			}

			maxExp := si.Activation + specpolicy.GetSectorMaxLifetime(si.SealProof, nv)
			if newExp > maxExp {
				newExp = maxExp
			}

			if newExp <= si.Expiration || withinTolerance(newExp, si.Expiration) {
				continue
			}

			l, found := activeSectorsLocation[si.SectorNumber]
			if !found {
				return fmt.Errorf("location for sector %d not found", si.SectorNumber)
			}

			es, found := extensions[*l]
			if !found {
				ne := make(map[abi.ChainEpoch][]abi.SectorNumber)
				ne[newExp] = []abi.SectorNumber{si.SectorNumber}
				extensions[*l] = ne
			} else {
				added := false
				for exp := range es {
					if withinTolerance(newExp, exp) {
						es[exp] = append(es[exp], si.SectorNumber)
						added = true
						break
					}
				}

				if !added {
					es[newExp] = []abi.SectorNumber{si.SectorNumber}
				}
			}
		}

		verifregAct, err := fapi.Chain.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to lookup verifreg actor: %w", err)
		}
		verifregSt, err := verifreg.Load(store, verifregAct)
		if err != nil {
			return fmt.Errorf("failed to load verifreg state: %w", err)
		}

		claimsMap, err := verifregSt.GetClaims(maddr)
		if err != nil {
			return fmt.Errorf("failed to lookup claims for miner: %w", err)
		}

		claimIdsBySector, err := getClaimIdsBySector(verifregSt, maddr)
		if err != nil {
			return fmt.Errorf("failed to lookup claim IDs by sector: %w", err)
		}

		sectorsMax, err := specpolicy.GetAddressedSectorsMax(nv)
		if err != nil {
			return err
		}

		declMax, err := specpolicy.GetDeclarationsMax(nv)
		if err != nil {
			return err
		}

		addrSectors := sectorsMax
		if cctx.Int("max-sectors") != 0 {
			addrSectors = cctx.Int("max-sectors")
			if addrSectors > sectorsMax {
				return fmt.Errorf("the specified max-sectors exceeds the maximum limit")
			}
		}

		var params []core.ExtendSectorExpiration2Params

		p := core.ExtendSectorExpiration2Params{}
		scount := 0

		for l, exts := range extensions {
			for newExp, numbers := range exts {

				sectorsWithoutClaimsToExtend := bitfield.New()
				var sectorsWithClaims []core.SectorClaim

				for _, sectorNumber := range numbers {
					claimIdsToMaintain := make([]verifreg.ClaimId, 0)
					claimIdsToDrop := make([]verifreg.ClaimId, 0)
					cannotExtendSector := false
					claimIds, ok := claimIdsBySector[sectorNumber]
					// Nothing to check, add to ccSectors
					if !ok {
						sectorsWithoutClaimsToExtend.Set(uint64(sectorNumber))
					} else {
						for _, claimID := range claimIds {
							claim, ok := claimsMap[claimID]
							if !ok {
								return fmt.Errorf("failed to find claim for claimId %d", claimID)
							}
							claimExpiration := claim.TermStart + claim.TermMax
							// can be maintained in the extended sector
							if claimExpiration > newExp {
								claimIdsToMaintain = append(claimIdsToMaintain, claimID)
							} else {
								sectorInfo, ok := activeSectorsInfo[sectorNumber]
								if !ok {
									return fmt.Errorf("failed to find sector in active sector set: %w", err)
								}
								if !cctx.Bool("drop-claims") ||
									// FIP-0045 requires the claim minimum duration to have passed
									currEpoch <= (claim.TermStart+claim.TermMin) ||
									// FIP-0045 requires the sector to be in its last 30 days of life
									(currEpoch <= sectorInfo.Expiration-builtin.EndOfLifeClaimDropPeriod) {
									fmt.Printf("skipping sector %d because claim %d does not live long enough \n", sectorNumber, claimID)
									cannotExtendSector = true
									break
								}

								claimIdsToDrop = append(claimIdsToDrop, claimID)
							}
						}
						if cannotExtendSector {
							continue
						}

						if len(claimIdsToMaintain)+len(claimIdsToDrop) != 0 {
							sectorsWithClaims = append(sectorsWithClaims, core.SectorClaim{
								SectorNumber:   sectorNumber,
								MaintainClaims: claimIdsToMaintain,
								DropClaims:     claimIdsToDrop,
							})
						}
					}
				}

				sectorsWithoutClaimsCount, err := sectorsWithoutClaimsToExtend.Count()
				if err != nil {
					return fmt.Errorf("failed to count cc sectors: %w", err)
				}

				sectorsInDecl := int(sectorsWithoutClaimsCount) + len(sectorsWithClaims)
				scount += sectorsInDecl

				if scount > addrSectors || len(p.Extensions) >= declMax {
					params = append(params, p)
					p = core.ExtendSectorExpiration2Params{}
					scount = sectorsInDecl
				}

				p.Extensions = append(p.Extensions, core.ExpirationExtension2{
					Deadline:          l.Deadline,
					Partition:         l.Partition,
					Sectors:           SectorNumsToBitfield(numbers),
					SectorsWithClaims: sectorsWithClaims,
					NewExpiration:     newExp,
				})
			}
		}

		// if we have any sectors, then one last append is needed here
		if scount != 0 {
			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := fapi.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		stotal := 0

		for i := range params {
			scount := 0
			for _, ext := range params[i].Extensions {
				count, err := ext.Sectors.Count()
				if err != nil {
					return err
				}
				scount += int(count)
			}
			fmt.Printf("Extending %d sectors: ", scount)
			stotal += scount

			if !cctx.Bool("really-do-it") {
				pp, err := NewPseudoExtendParams(&params[i])
				if err != nil {
					return err
				}

				data, err := json.MarshalIndent(pp, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println("\n", string(data))
				continue
			}

			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return fmt.Errorf("serializing params: %w", err)
			}

			msg := &types.Message{
				From: mi.Worker,

				To:     maddr,
				Method: stbuiltin.MethodsMiner.ExtendSectorExpiration2,
				Params: sp,

				Value: big.Zero(),
			}

			mid, err := fapi.Messager.PushMessage(ctx, msg, spec)
			if err != nil {
				return RPCCallError("PushMessageWithId", err)
			}

			fmt.Println(mid)
		}

		fmt.Printf("%d sectors extended\n", stotal)

		return nil
	},
}

// TODO: this function should remove after Venus has released the `venus_shared/actors/builtin/verifreg.State.GetClaimIdsBySector` method
func getClaimIdsBySector(s verifreg.State, providerIDAddr address.Address) (map[abi.SectorNumber][]verifreg.ClaimId, error) {

	claims, err := s.GetClaims(providerIDAddr)

	retMap := make(map[abi.SectorNumber][]verifreg.ClaimId)
	for k, v := range claims {
		claims, ok := retMap[v.Sector]
		if !ok {
			retMap[v.Sector] = []verifreg.ClaimId{k}
		} else {
			retMap[v.Sector] = append(claims, k)
		}
	}

	return retMap, err
}

var utilSealerSectorsTerminateCmd = &cli.Command{
	Name:      "terminate",
	Usage:     "Terminate sector on-chain (WARNING: This means losing power and collateral for the sector)",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "actor",
			Required: true,
			Usage:    "actor id, eg. 1000",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Subcommands: []*cli.Command{
		utilSealerSectorsTerminateQueryCmd,
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("pass --really-do-it to confirm this action")
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		resp, err := cli.Sealer.TerminateSector(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		if resp.Res != core.SubmitAccepted {
			fmt.Printf("terminate failed: %s\n", *resp.Desc)
		}
		fmt.Println("terminate accepted")

		return nil
	},
}

var utilSealerSectorsTerminateQueryCmd = &cli.Command{
	Name:      "query",
	Usage:     "Query the terminate info of the specified sector",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		resp, err := cli.Sealer.PollTerminateSectorState(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		if resp.TerminateCid != nil {
			fmt.Printf("msg: %s, height: %v, added height: %v\n", resp.TerminateCid.String(), resp.TerminatedAt, resp.AddedHeight)
		} else {
			fmt.Printf("msg: null, added height: %v\n", resp.AddedHeight)
		}

		return nil
	},
}

var utilSealerSectorsRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "Forcefully remove persist stores of sector(WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "actor",
			Required: true,
			Usage:    "actor id, eg. 1000",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("pass --really-do-it to confirm this action")
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		err = cli.Sealer.RemoveSector(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		fmt.Println("remove succeed")
		return nil
	},
}

var utilSealerSectorsFinalizeCmd = &cli.Command{
	Name:      "finalize",
	Usage:     "Mandatory label the sector status as the finalize, this is only to the sector that has been on the chain.",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "actor",
			Required: true,
			Usage:    "actor id, eg. 1000",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("pass --really-do-it to confirm this action")
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		err = cli.Sealer.FinalizeSector(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		fmt.Println("finalize succeed")
		return nil
	},
}

var utilSealerSectorsStateCmd = &cli.Command{
	Name:      "state",
	Usage:     "Load and display the detailed sector state",
	ArgsUsage: "<minerID> <sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "offline",
			Usage: "DEPRECATED: damocles-manager will load sector state in all state dbs",
		},
	},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		minerID, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return err
		}

		sectorNumber, err := ShouldSectorNumber(args.Get(1))
		if err != nil {
			return err
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		sid := abi.SectorID{
			Miner:  minerID,
			Number: sectorNumber,
		}

		state, err := cli.Sealer.FindSectorInAllStates(gctx, sid)
		if err != nil {
			return RPCCallError("FindSectorInAllStates", err)
		}

		fmt.Fprintf(os.Stdout, "Sector %s: \n", util.FormatSectorID(sid))
		// Common
		fmt.Fprintln(os.Stdout, "\nCommon:")
		fmt.Fprintf(os.Stdout, "\tFinalized: %v\n", state.Finalized)
		fmt.Fprintf(os.Stdout, "\tRemoved: %v\n", state.Removed)
		abortReason := state.AbortReason
		if abortReason == "" {
			abortReason = "NULL"
		}
		fmt.Fprintf(os.Stdout, "\tAborting: \n\t\t%s\n", strings.ReplaceAll(abortReason, "\n", "\n\t\t"))

		// LatestState
		fmt.Fprintln(os.Stdout, "\nLatestState:")
		fmt.Fprintf(os.Stdout, "\tState Change: %s\n", FormatOrNull(state.LatestState, func() string {
			return fmt.Sprintf("%s => %s, by %s", state.LatestState.StateChange.Prev, state.LatestState.StateChange.Next, state.LatestState.StateChange.Event)
		}))
		fmt.Fprintf(os.Stdout, "\tWorker: %s\n", FormatOrNull(state.LatestState, func() string {
			return fmt.Sprintf("%s(%s)", state.LatestState.Worker.Instance, state.LatestState.Worker.Location)
		}))
		fmt.Fprintf(os.Stdout, "\tFailure: %s\n", FormatOrNull(state.LatestState, func() string {
			return FormatOrNull(state.LatestState.Failure, func() string {
				return fmt.Sprintf("\n\t\t[%s] %s", state.LatestState.Failure.Level, strings.ReplaceAll(state.LatestState.Failure.Desc, "\n", "\n\t\t"))
			})
		}))

		// Deals
		fmt.Fprintln(os.Stdout, "\nDeals:")
		deals := state.Deals()
		if len(deals) == 0 {
			fmt.Fprintln(os.Stdout, "\tNULL")
		} else {
			for _, deal := range deals {
				fmt.Fprintf(os.Stdout, "\tID: %d\n", deal.ID)
				fmt.Fprintf(os.Stdout, "\tPiece: %v\n", deal.Piece)
			}
		}

		// Sealing
		fmt.Fprintln(os.Stdout, "\nSealing:")
		fmt.Fprintf(os.Stdout, "\tTicket: %s\n", FormatOrNull(state.Ticket, func() string {
			return fmt.Sprintf("(%d) %x", state.Ticket.Epoch, state.Ticket.Ticket)
		}))

		fmt.Fprintf(os.Stdout, "\tPreCommit Info:\n\t\t%s\n", FormatOrNull(state.Pre, func() string {
			return fmt.Sprintf("CommD: %s\n\t\tCommR: %s", state.Pre.CommD, state.Pre.CommR)
		}))

		fmt.Fprintf(os.Stdout, "\tPreCommit Message: %s\n", FormatOrNull(state.MessageInfo.PreCommitCid, func() string {
			return state.MessageInfo.PreCommitCid.String()
		}))

		fmt.Fprintf(os.Stdout, "\tSeed: %s\n", FormatOrNull(state.Seed, func() string {
			return fmt.Sprintf("(%d) %x", state.Seed.Epoch, state.Seed.Seed)
		}))

		fmt.Fprintf(os.Stdout, "\tProveCommit Info:\n\t\t%s\n", FormatOrNull(state.Proof, func() string {
			return fmt.Sprintf("Proof: %x", state.Proof.Proof)
		}))

		fmt.Fprintf(os.Stdout, "\tProveCommit Message: %s\n", FormatOrNull(state.MessageInfo.CommitCid, func() string {
			return state.MessageInfo.CommitCid.String()
		}))

		fmt.Fprintf(os.Stdout, "\tMessage NeedSend: %v\n", state.MessageInfo.NeedSend)

		// Upgrading
		fmt.Fprintln(os.Stdout, "\nSnapUp:")
		fmt.Fprintf(os.Stdout, "\tUpgraded: %v\n", state.Upgraded)
		if state.Upgraded {
			if state.UpgradedInfo != nil {
				fmt.Fprintf(os.Stdout, "\tUnsealedCID: %s\n", state.UpgradedInfo.UnsealedCID)
				fmt.Fprintf(os.Stdout, "\tSealedCID: %s\n", state.UpgradedInfo.UnsealedCID)
				fmt.Fprintf(os.Stdout, "\tProof: %x\n", state.UpgradedInfo.Proof[:])
			}

			if state.UpgradeMessageID != nil {
				fmt.Fprintf(os.Stdout, "\tUpgrade Message: %s\n", *state.UpgradeMessageID)
			}

			if state.UpgradeLandedEpoch != nil {
				fmt.Fprintf(os.Stdout, "\tLanded Epoch: %d\n", *state.UpgradeLandedEpoch)
			}
		}

		// Termination
		fmt.Fprintln(os.Stdout, "\nTermination:")
		fmt.Fprintf(os.Stdout, "\tTerminate Message: %s\n", FormatOrNull(state.TerminateInfo.TerminateCid, func() string {
			return state.TerminateInfo.TerminateCid.String()
		}))

		// Rebuild
		fmt.Fprintf(os.Stdout, "\nRebuild: %v\n", state.NeedRebuild)

		fmt.Fprintln(os.Stdout, "")

		return nil
	},
}

var utilSealerSectorsFindDealCmd = &cli.Command{
	Name:      "find-deal",
	Usage:     "Find the sectors to which the deal was assigned",
	ArgsUsage: "<dealID>",
	Flags: []cli.Flag{
		flagListOffline,
	},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		dealID, err := strconv.ParseUint(args.First(), 10, 64)
		if err != nil {
			return fmt.Errorf("parse deal id: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		sectors, err := cli.Sealer.FindSectorsWithDeal(gctx, extractListWorkerState(cctx), abi.DealID(dealID))
		if err != nil {
			return RPCCallError("FindSectorsWithDeal", err)
		}

		if len(sectors) == 0 {
			fmt.Fprintln(os.Stdout, "Not Found")
			return nil
		}

		for _, sector := range sectors {
			fmt.Fprintln(os.Stdout, util.FormatSectorID(sector.ID))
		}

		fmt.Fprintln(os.Stdout, "")

		return nil
	},
}

var utilSealerSectorsResendPreCommitCmd = &cli.Command{
	Name:      "resend-pre",
	Usage:     "Resend the pre commit on chain info for the specified sector, should only be used in situations that won't recover automatically",
	ArgsUsage: "<minerID> <sectorNum>",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		minerID, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return err
		}

		sectorNumber, err := ShouldSectorNumber(args.Get(1))
		if err != nil {
			return err
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		sid := abi.SectorID{
			Miner:  minerID,
			Number: sectorNumber,
		}

		state, err := cli.Sealer.FindSector(gctx, core.WorkerOnline, sid)
		if err != nil {
			return RPCCallError("FindSector", err)
		}

		if state.Proof != nil {
			return fmt.Errorf("the sector has been reached later stages, unable to resend")
		}

		if state.Pre == nil {
			return fmt.Errorf("no pre commit on chain info available")
		}

		if state.MessageInfo.NeedSend {
			return fmt.Errorf("sector is still being marked as 'Need To Be Send' in the state machine")
		}

		onChainInfo, err := state.Pre.IntoPreCommitOnChainInfo()
		if err != nil {
			return fmt.Errorf("convert to pre commit on chain info: %w", err)
		}

		resp, err := cli.Sealer.SubmitPreCommit(gctx, core.AllocatedSector{
			ID:        sid,
			ProofType: state.SectorType,
		}, onChainInfo, true)

		if err != nil {
			return RPCCallError("SubmitPreCommit", err)
		}

		if resp.Res != core.SubmitAccepted {
			return fmt.Errorf("unexpected submit result: %d, err: %s", resp.Res, FormatOrNull(resp.Desc, func() string {
				return *resp.Desc
			}))
		}

		Log.Info("pre commit on chain info reset")
		return nil
	},
}

var utilSealerSectorsResendProveCommitCmd = &cli.Command{
	Name:      "resend-prove",
	Usage:     "Resend the prove commit on chain info for the specified sector, should only be used in situations that won't recover automatically",
	ArgsUsage: "<minerID> <sectorNum>",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		minerID, err := ShouldActor(args.Get(0), true)
		if err != nil {
			return err
		}

		sectorNumber, err := ShouldSectorNumber(args.Get(1))
		if err != nil {
			return err
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		sid := abi.SectorID{
			Miner:  minerID,
			Number: sectorNumber,
		}

		state, err := cli.Sealer.FindSector(gctx, core.WorkerOnline, sid)
		if err != nil {
			return RPCCallError("FindSector", err)
		}

		if state.Proof == nil {
			return fmt.Errorf("no prove commit on chain info available")
		}

		if state.MessageInfo.NeedSend {
			return fmt.Errorf("sector is still being marked as 'Need To Be Send' in the state machine")
		}

		resp, err := cli.Sealer.SubmitProof(gctx, sid, *state.Proof, true)

		if err != nil {
			return RPCCallError("SubmitProof", err)
		}

		if resp.Res != core.SubmitAccepted {
			return fmt.Errorf("unexpected submit result: %d, err: %s", resp.Res, FormatOrNull(resp.Desc, func() string {
				return *resp.Desc
			}))
		}

		Log.Info("prove commit on chain info reset")
		return nil
	},
}

var utilSealerSectorsImportCmd = &cli.Command{
	Name:  "import",
	Usage: "Import sector infos from the given lotus-miner instance",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api",
			Usage: "api address of the instance",
		},
		&cli.StringFlag{
			Name:  "token",
			Usage: "api token of the instance",
		},
		&cli.BoolFlag{
			Name:  "override",
			Usage: "override the previous sector state",
			Value: false,
		},
		&cli.Uint64SliceFlag{
			Name:  "numbers",
			Usage: "import the specified sector numbers only if this flag is set",
		},
	},
	Action: func(cctx *cli.Context) error {
		override := cctx.Bool("override")

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		mcli, closer, err := lotusminer.New(gctx, cctx.String("api"), cctx.String("token"))
		if err != nil {
			return fmt.Errorf("construct lotus-miner client: %w", err)
		}

		defer closer()

		maddr, err := mcli.ActorAddress(gctx)
		if err != nil {
			return RPCCallError("ActorAddress", err)
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return fmt.Errorf("extract actor id from %s: %w", maddr, err)
		}

		minerID := abi.ActorID(mid)

		var numbers []abi.SectorNumber
		if cctx.IsSet("numbers") {
			nums := cctx.Uint64Slice("numbers")
			numbers = make([]abi.SectorNumber, 0, len(nums))
			for _, num := range nums {
				numbers = append(numbers, abi.SectorNumber(num))
			}
		} else {
			numbers, err = mcli.SectorsList(gctx)
			if err != nil {
				return RPCCallError("SectorsList", err)
			}
		}

		for _, num := range numbers {
			sinfo, err := mcli.SectorsStatus(gctx, num, true)
			if err != nil {
				return RPCCallError("SectorsStatus", fmt.Errorf("get info for %d: %w", num, err))
			}

			sid := abi.SectorID{
				Miner:  minerID,
				Number: sinfo.SectorID,
			}

			slog := Log.With("sector", util.FormatSectorID(sid))
			state, err := sectorInfo2SectorState(sid, &sinfo)
			if err != nil {
				slog.Warnf("check sector info: %s", err)
				continue
			}

			imported, err := cli.Sealer.ImportSector(gctx, core.WorkerOffline, state, override)
			if err != nil {
				slog.Errorf("import failed: %s", err)
				continue
			}

			if !imported {
				slog.Warn("not imported")
				continue
			}

			slog.Info("imported")
		}

		return nil
	},
}

var utilSealerSectorsExportCmd = &cli.Command{
	Name:  "export",
	Usage: "Commands for export sector infos to the given lotus-miner instance",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "miner",
			Required: true,
		},
	},
	Subcommands: []*cli.Command{
		utilSealerSectorsExportMetadataCmd,
		utilSealerSectorsExportFilesCmd,
	},
}

var utilSealerSectorsExportMetadataCmd = &cli.Command{
	Name:  "metadata",
	Usage: "Export metadata of the sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "dest-repo",
			Value: "~/.lotusminer",
		},
		&cli.Uint64Flag{
			Name:  "next-number",
			Usage: "Specify the sector number of the new sector",
		},
		&cli.BoolFlag{
			Name:  "only-next-number",
			Usage: "Only update the next sector number",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		onlyNextSid := cctx.Bool("only-next-number")
		if onlyNextSid {
			if !cctx.IsSet("next-number") {
				return fmt.Errorf("flag next-number must be set when only update the value of it")
			}
		}

		destRepo := cctx.String("dest-repo")
		ds, err := openDestDatastore(destRepo)
		if err != nil {
			return fmt.Errorf("open datastore %s: %w", destRepo, err)
		}
		defer ds.Close()

		nextSid := abi.SectorNumber(0)
		ctx := cctx.Context

		if !onlyNextSid {
			minerID, err := ShouldActor(cctx.String("miner"), true)
			if err != nil {
				return fmt.Errorf("invalid miner actor id: %w", err)
			}

			cli, gctx, stop, err := extractAPI(cctx)
			if err != nil {
				return err
			}
			defer stop()

			states, err := cli.Sealer.ListSectors(gctx, core.WorkerOffline, core.SectorWorkerJobAll)
			if err != nil {
				return err
			}

			failCounts := 0
			for _, state := range states {
				if state.ID.Miner != minerID {
					continue
				}

				if state.NeedRebuild { // todo skip need-rebuild?
					continue
				}

				// Skip the existing sector, which may be sealed before switching
				sectorKey := datastore.NewKey(lotusminer.SectorStorePrefix).ChildString(fmt.Sprint(state.ID.Number))
				has, _ := ds.Has(ctx, sectorKey)
				if has {
					continue
				}

				sector, err := sectorState2SectorInfo(ctx, cli, state)
				if err != nil {
					fmt.Fprintf(os.Stdout, "sector %v to sector info err: %s\n", state.ID.Number, err)
					failCounts++
				} else {
					buf, err := cborutil.Dump(sector)
					if err != nil {
						fmt.Fprintf(os.Stdout, "cborutil dump sector %v err: %s\n", state.ID.Number, err)
						failCounts++
					} else {
						if err = ds.Put(ctx, sectorKey, buf); err != nil {
							fmt.Fprintf(os.Stdout, "put sector %v err: %s\n", state.ID.Number, err)
							failCounts++
						}
					}
				}

				if nextSid < state.ID.Number {
					nextSid = state.ID.Number
				}
			}
			fmt.Fprintf(os.Stdout, "export failure counts: %d\n", failCounts)
		}

		// export the sector number of the new sector
		if cctx.IsSet("next-number") {
			nextNumber := cctx.Uint64("next-number")
			fmt.Fprintf(os.Stdout, "next sector number: %d\n", nextNumber)
			if nextSid < abi.SectorNumber(nextNumber) {
				nextSid = abi.SectorNumber(nextNumber)
			}
		}
		buf := make([]byte, binary.MaxVarintLen64)
		size := binary.PutUvarint(buf, uint64(nextSid))
		err = ds.Put(ctx, datastore.NewKey(lotusminer.StorageCounterDSPrefix), buf[:size])
		if err != nil {
			return fmt.Errorf("update next sector number: %v", err)
		}
		fmt.Fprintf(os.Stdout, "update next sector number: %d\n", nextSid)

		return nil
	},
}

var utilSealerSectorsExportFilesCmd = &cli.Command{
	Name:  "files",
	Usage: "Export the files of the sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "dest-path",
			Required: true,
		},
		&cli.Uint64SliceFlag{
			Name:  "numbers",
			Usage: "Export the specified sector numbers only if this flag is set",
		},
		&cli.BoolFlag{
			Name:  "reserve",
			Usage: "Reserve source files",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		destPath := cctx.String("dest-path")
		absPath, err := filepath.Abs(destPath)
		if err != nil {
			return fmt.Errorf("get abs path: %w", err)
		}

		ctx := cctx.Context

		minerID, err := ShouldActor(cctx.String("miner"), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		numbers := make(map[abi.SectorNumber]struct{})
		if cctx.IsSet("numbers") {
			nums := cctx.Uint64Slice("numbers")
			for _, num := range nums {
				numbers[abi.SectorNumber(num)] = struct{}{}
			}
		}

		find := func(number abi.SectorNumber) bool {
			_, ok := numbers[number]
			return ok
		}

		fileCompare := func(srcPath, dstPath string) (bool, error) {
			// only check file szie
			srcStat, err := os.Stat(srcPath)
			if err != nil {
				return false, err
			}
			dstStat, err := os.Stat(dstPath)
			if err != nil {
				return false, err
			}
			return srcStat.Size() == dstStat.Size(), nil
		}

		copyFile := func(srcPath, dstPath string) error {
			src, err := os.Open(srcPath)
			if err != nil {
				return err
			}
			defer src.Close()

			dst, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			defer dst.Close()

			buf := make([]byte, 1024*1024) // 1M
			_, err = io.CopyBuffer(dst, src, buf)
			if err != nil {
				return err
			}

			return nil
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, core.WorkerOffline, core.SectorWorkerJobAll)
		if err != nil {
			return err
		}

		failCounts, totalCunts := 0, 0
		var lockCts sync.Mutex
		var wg sync.WaitGroup
		reserve := cctx.Bool("reserve")
		throttle := make(chan struct{}, 5)
		for _, state := range states {
			if state.ID.Miner != minerID {
				continue
			}

			if len(numbers) > 0 && !find(state.ID.Number) {
				continue
			}

			loc, err := cli.Sealer.ProvingSectorInfo(ctx, state.ID)
			if err != nil {
				fmt.Fprintf(os.Stdout, "find sector %v location: %s\n", state.ID.Number, err)
				failCounts++
				continue
			}

			// move
			fmt.Fprintf(os.Stdout, "move sector %d file ...\n", state.ID.Number)
			totalCunts++
			wg.Add(1)
			throttle <- struct{}{}
			go func(sid abi.SectorNumber) {
				defer wg.Done()
				defer func() {
					<-throttle
				}()

				for _, dir := range []string{loc.Private.SealedSectorPath, loc.Private.CacheDirPath} {
					if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return err
						}

						if !info.IsDir() && !strings.Contains(info.Name(), ".json") {
							destPath := ""
							if dir == loc.Private.SealedSectorPath {
								destPath = filepath.Join(absPath, loc.Private.SealedSectorURI)
							} else {
								destPath = filepath.Join(absPath, loc.Private.CacheDirURI, info.Name())
							}

							if reserve {
								needMove := false
								_, err := os.Stat(destPath)
								if err != nil {
									if os.IsNotExist(err) {
										needMove = true
									} else {
										fmt.Fprintf(os.Stdout, "stat path %s: %s", destPath, err)
									}
								} else {
									if equal, _ := fileCompare(path, destPath); !equal {
										needMove = true
									}
								}

								if needMove {
									fmt.Fprintf(os.Stdout, "copy file from %s to %s\n", path, destPath)
									err = copyFile(path, destPath)
									if err != nil {
										return fmt.Errorf("copy %s to %s err: %w", path, destPath, err)
									}
								}

							} else {
								fmt.Fprintf(os.Stdout, "move file from %s to %s\n", path, destPath)
								err = os.Rename(path, destPath)
								if err != nil {
									return fmt.Errorf("move %s to %s err: %w", path, destPath, err)
								}
							}
						} else {
							if err := os.MkdirAll(filepath.Join(absPath, loc.Private.CacheDirURI), 0755); err != nil {
								return err
							}
						}
						return nil
					}); err != nil {
						fmt.Fprintf(os.Stdout, "export sector %v file: %s\n", sid, err)
						lockCts.Lock()
						failCounts++
						lockCts.Unlock()
						return
					}
				}
			}(state.ID.Number)
		}
		wg.Wait()
		fmt.Fprintf(os.Stdout, "export failure counts: %d, total: %d\n", failCounts, totalCunts)

		return nil
	},
}

func sectorInfo2SectorState(sid abi.SectorID, sinfo *lotusminer.SectorInfo) (*core.SectorState, error) {
	var upgraded core.SectorUpgraded
	switch lotusminer.SectorState(sinfo.State) {
	case lotusminer.FinalizeSector, lotusminer.Proving:

	case lotusminer.FinalizeReplicaUpdate,
		lotusminer.UpdateActivating,
		lotusminer.ReleaseSectorKey:
		upgraded = true

	default:
		return nil, fmt.Errorf("unexpected sector state %s", sinfo.State)
	}

	if sinfo.CommD == nil {
		return nil, fmt.Errorf("no comm_d")
	}

	if sinfo.CommR == nil {
		return nil, fmt.Errorf("no comm_r")
	}

	commR, err := util.CID2ReplicaCommitment(*sinfo.CommR)
	if err != nil {
		return nil, fmt.Errorf("convert comm_r cid to commitment: %w", err)
	}

	if len(sinfo.Proof) == 0 {
		return nil, fmt.Errorf("no proof")
	}

	if len(sinfo.Ticket.Value) == 0 {
		return nil, fmt.Errorf("no ticket")
	}

	if len(sinfo.Seed.Value) == 0 {
		return nil, fmt.Errorf("no seed")
	}

	ticket := core.Ticket{
		Epoch:  sinfo.Ticket.Epoch,
		Ticket: abi.Randomness(sinfo.Ticket.Value),
	}

	// pieces
	pieces := make(core.Deals, 0, len(sinfo.Pieces))
	dealIDs := make([]abi.DealID, 0, len(sinfo.Pieces))
	for pi := range sinfo.Pieces {
		ipiece := sinfo.Pieces[pi]
		spiece := core.DealInfo{}

		if ipiece.DealInfo != nil {
			if ipiece.DealInfo.DealID != 0 {
				dealIDs = append(dealIDs, ipiece.DealInfo.DealID)
				spiece.ID = ipiece.DealInfo.DealID
				spiece.IsCompatible = true
			}

			spiece.Piece = core.PieceInfo{
				Size: ipiece.Piece.Size,
				Cid:  ipiece.Piece.PieceCID,
			}
			spiece.Proposal = ipiece.DealInfo.DealProposal
		}

		pieces = append(pieces, spiece)
	}

	if len(dealIDs) == 0 {
		pieces = pieces[:0]
	}

	state := &core.SectorState{
		ID:         sid,
		SectorType: sinfo.SealProof,

		Ticket: &ticket,
		Seed: &core.Seed{
			Epoch: sinfo.Seed.Epoch,
			Seed:  abi.Randomness(sinfo.Seed.Value),
		},
		Pieces: pieces,
		Pre: &core.PreCommitInfo{
			CommR:  *sinfo.CommR,
			CommD:  *sinfo.CommD,
			Ticket: ticket,
			Deals:  dealIDs,
		},
		Proof: &core.ProofInfo{
			Proof: sinfo.Proof,
		},

		MessageInfo: core.MessageInfo{
			PreCommitCid: sinfo.PreCommitMsg,
			CommitCid:    sinfo.CommitMsg,
		},

		Finalized: true,

		Upgraded: upgraded,

		Imported: true,
	}

	if upgraded {
		state.UpgradePublic = &core.SectorUpgradePublic{
			CommR:      commR,
			SealedCID:  *sinfo.CommR,
			Activation: sinfo.Activation,
			Expiration: sinfo.Expiration,
		}

		// 这个零值行为在重建扇区时会作为判断依据，一旦改变，需要同步修正
		state.UpgradedInfo = &core.SectorUpgradedInfo{}

		if sinfo.ReplicaUpdateMessage != nil {
			msgID := core.SectorUpgradeMessageID(sinfo.ReplicaUpdateMessage.String())
			state.UpgradeMessageID = &msgID
		}

		var landedEpoch core.SectorUpgradeLandedEpoch
		state.UpgradeLandedEpoch = &landedEpoch
	}

	return state, nil
}

func openDestDatastore(repoPath string) (datastore.Batching, error) {
	path := filepath.Join(repoPath, "datastore", "metadata")
	return levelds.NewDatastore(path, &levelds.Options{
		Compression: ldbopts.NoCompression,
		NoSync:      false,
		Strict:      ldbopts.StrictAll,
		ReadOnly:    false,
	})
}

func sectorState2SectorInfo(ctx context.Context, api *APIClient, state *core.SectorState) (*lotusminer.SectorSealingInfo, error) {
	var toChainCid = func(mid string) *cid.Cid {
		undefCid := cid.NewCidV0(u.Hash([]byte("undef")))
		c := &undefCid
		if len(mid) > 0 {
			if msg, err := api.Messager.GetMessageByUid(ctx, mid); err == nil {
				c = msg.SignedCid
			}
		}

		return c
	}

	unKnownCid := func() *cid.Cid {
		c, _ := cid.Decode("bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve")
		return &c
	}

	sector := lotusminer.SectorSealingInfo{}
	sector.State = lotusminer.Proving
	sector.SectorNumber = state.ID.Number

	sector.SectorType = state.SectorType

	// Packing
	// sector.CreationTime = 0
	pieces := make([]lotusminer.SectorPiece, 0)
	for pi := range state.Pieces {
		piece := state.Pieces[pi]
		pieces = append(pieces, lotusminer.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     piece.Piece.Size,
				PieceCID: piece.Piece.Cid,
			},
			DealInfo: &lotusminer.PieceDealInfo{
				PublishCid:   unKnownCid(),
				DealID:       piece.ID,
				DealProposal: piece.Proposal,
			},
		})
	}
	sector.Pieces = pieces

	// PreCommit1
	if state.Ticket != nil {
		sector.TicketValue = abi.SealRandomness(state.Ticket.Ticket)
		sector.TicketEpoch = state.Ticket.Epoch
	}
	sector.PreCommit1Out = []byte("")

	// PreCommit2
	if state.Pre != nil {
		sector.CommD = &state.Pre.CommD
		sector.CommR = &state.Pre.CommR
	}
	if state.Proof != nil {
		sector.Proof = state.Proof.Proof
	}
	if state.MessageInfo.CommitCid != nil {
		sector.PreCommitMessage = toChainCid(state.MessageInfo.PreCommitCid.String())
	}

	// WaitSeed
	if state.Seed != nil {
		sector.SeedValue = abi.InteractiveSealRandomness(state.Seed.Seed)
		sector.SeedEpoch = state.Seed.Epoch
	}

	// Committing
	if state.MessageInfo.CommitCid != nil {
		sector.CommitMessage = toChainCid(state.MessageInfo.CommitCid.String())
	}
	sector.InvalidProofs = 0

	// CCUpdate
	sector.CCUpdate = bool(state.Upgraded)
	if state.Upgraded && state.UpgradedInfo != nil {
		sector.UpdateSealed = &state.UpgradedInfo.SealedCID
		sector.UpdateUnsealed = &state.UpgradedInfo.UnsealedCID
		sector.ReplicaUpdateProof = state.UpgradedInfo.Proof
		mid := *state.UpgradeMessageID
		sector.ReplicaUpdateMessage = toChainCid(string(mid))
	}

	// Termination
	if state.TerminateInfo.TerminateCid != nil {
		sector.TerminateMessage = toChainCid(state.TerminateInfo.TerminateCid.String())
		sector.TerminatedAt = state.TerminateInfo.TerminatedAt
	}

	return &sector, nil
}

var utilSealerSectorsRebuildCmd = &cli.Command{
	Name:      "rebuild",
	Usage:     "Rebuild specified sector",
	ArgsUsage: "<miner actor> <sector number>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "pieces-available",
			Usage: "if all pieces are available in venus-market, this flag is used for imported sectors",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.SectorSetForRebuild(gctx, abi.SectorID{
			Miner:  miner,
			Number: abi.SectorNumber(sectorNum),
		}, core.RebuildOptions{
			PiecesAvailable: cctx.Bool("pieces-available"),
		})
		if err != nil {
			return fmt.Errorf("set sector for rebuild failed: %w", err)
		}

		return nil
	},
}

var utilSealerSectorsUnsealCmd = &cli.Command{
	Name:      "unseal",
	Usage:     "unseal specified sector",
	ArgsUsage: "<piece_cid>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Usage:   "output piece as a car file to the specific path",
			Aliases: []string{"o"},
		},
		&cli.Uint64Flag{
			Name:     "actor",
			Usage:    "specify actor id of miner manully, it must worke with flag \"--sector\" ",
			Required: false,
			Aliases:  []string{"miner", "actor-id"},
		},
		&cli.Uint64Flag{
			Name:     "sector",
			Usage:    "specify sector number manully, it must worke with flag \"--actor\" ",
			Aliases:  []string{"sector-id"},
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "piece-info-from-droplet",
			Usage:    "get piece info from droplet, which come from damocles db by default .",
			Value:    false,
			Aliases:  []string{"from-droplet"},
			Required: false,
		},
		&cli.StringFlag{
			Name:     "unseal-file",
			Usage:    "unseal piece from unseal file",
			Required: false,
		},
		&cli.Uint64Flag{
			Name:     "offset",
			Usage:    "specify offset of piece manually",
			Value:    0,
			Required: false,
		},
		&cli.Uint64Flag{
			Name:     "size",
			Usage:    "specify size of piece manually",
			Value:    0,
			Required: false,
		},
		&cli.StringFlag{
			Name:     "dest",
			Usage:    "specify destination to transfer piece manually, there are five protocols can be used:" + "\"file:///path\",\"http://\" \"https://\", \"market://store_name/piece_cid\", \"store://store_name/piece_cid\"",
			Required: false,
		},
	},
	Action: func(cctx *cli.Context) error {

		if count := cctx.Args().Len(); count < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		pieceCid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("invalid piece cid: %w", err)
		}

		// get sector id
		sectorID := abi.SectorID{}
		// todo: rm sectorState when we can get deal by dealID
		var sectorState *core.SectorState
		if cctx.IsSet("actor") && cctx.IsSet("sector") {
			miner := abi.ActorID(cctx.Uint64("actor"))
			sector := abi.SectorNumber(cctx.Uint64("sector"))
			sectorID = abi.SectorID{
				Miner:  miner,
				Number: sector,
			}
			sectorState, err = cli.Sealer.FindSectorInAllStates(gctx, sectorID)
			if err != nil {
				return fmt.Errorf("get sector info failed: %w", err)
			}
		} else if cctx.IsSet("actor") || cctx.IsSet("sector") {
			return fmt.Errorf("flag \"--actor\" and \"--sector\" must be set together")
		} else {
			sector, err := cli.Sealer.FindSectorWithPiece(gctx, core.WorkerOffline, pieceCid)
			if err != nil {
				return fmt.Errorf("find sector with piece: %w", err)
			}

			if sector == nil {
				return fmt.Errorf("no sector found with piece %s", pieceCid)
			}
			sectorState = sector
		}

		// get piece-info
		var offsetPadded, sizePadded abi.PaddedPieceSize
		if cctx.Bool("piece-info-from-droplet") {
			// get piece info from market
			pieceInfo, err := cli.Market.PiecesGetPieceInfo(gctx, pieceCid)
			if err != nil {
				return fmt.Errorf("get piece info from market: %w", err)
			}
			if pieceInfo == nil {
				return fmt.Errorf("no piece info found in market with piece %s", pieceCid)
			}
			var matched bool
			for _, deal := range pieceInfo.Deals {
				// check miner and sector
				for _, dealInSectorState := range sectorState.Pieces {
					if deal.DealID == dealInSectorState.ID {
						matched = true
						offsetPadded = deal.Offset
						sizePadded = deal.Length
						break
					}
				}
			}
			if !matched {
				return fmt.Errorf("no matched deal found in market with sector %d and piece %s", sectorState.ID.Number, pieceCid)
			}
		} else {
			// get piece info from sector state
			for _, p := range sectorState.Pieces {
				if pieceCid.Equals(p.Piece.Cid) {
					offsetPadded = p.Piece.Offset
					sizePadded = p.Piece.Size
					break
				}
			}
		}

		// allow cover offset and size by flag
		if cctx.IsSet("offset") {
			offsetPadded = abi.PaddedPieceSize(cctx.Uint64("offset"))
		}
		if cctx.IsSet("size") {
			sizePadded = abi.PaddedPieceSize(cctx.Uint64("size"))
		}

		offset := offsetPadded.Unpadded()
		size := sizePadded.Unpadded()

		if err := offset.Validate(); err != nil && offset != 0 {
			return fmt.Errorf("invalid offset: %w", err)
		}
		if err := size.Validate(); err != nil {
			return fmt.Errorf("invalid size: %w", err)
		}

		// unseal by vsm or worker
		output := cctx.String("output")
		if output == "" {
			pwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get pwd failed: %w", err)
			}
			output = fmt.Sprintf("%s/%s", pwd, pieceCid.String())
		}

		dest := cctx.String("dest")
		if cctx.IsSet("unseal-file") {
			if cctx.IsSet("dest") {
				return fmt.Errorf("flag \"--unseal-file\" and \"--dest\" can not be set together")
			}
			unsealFile := cctx.String("unseal-file")
			fi, err := os.Stat(unsealFile)
			if err != nil {
				return fmt.Errorf("check unseal file failed: %w", err)
			}
			if fi.IsDir() {
				return fmt.Errorf("unseal file can not be a directory")
			}

			src, err := os.ReadFile(unsealFile)
			if err != nil {
				return fmt.Errorf("read unseal file failed: %w", err)
			}

			data := src[offsetPadded : offsetPadded+sizePadded]
			res := make([]byte, size)

			fr32.Unpad(data, res)

			err = os.WriteFile(output, res, 0644)
			if err != nil {
				return fmt.Errorf("write piece file failed: %w", err)
			}

		} else {
			stream, err := cli.Sealer.UnsealPiece(gctx, sectorID, pieceCid, types.UnpaddedByteIndex(offset), size, dest)
			if err != nil {
				return fmt.Errorf("set task for unseal failed: %w", err)
			}

			if stream != nil {

				fi, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer func() {
					err := fi.Close()
					if err != nil {
						fmt.Printf("error closing output file: %+v", err)
					}
				}()

				var finish bool
				for b := range stream {
					finish = len(b) == 0
					fmt.Printf("unseal piece bytes: %x \n", b)
					_, err := fi.Write(b)
					if err != nil {
						return err
					}
				}

				if !finish {
					return fmt.Errorf("unseal piece failed")
				}
			}
		}

		fmt.Println("set task for unseal success:")
		fmt.Printf("piece cid: %s\n", pieceCid)
		fmt.Printf("offset: %d\n", offsetPadded)
		fmt.Printf("size: %d\n", sizePadded)
		fmt.Printf("dest: %s\n", dest)

		return nil
	},
}
