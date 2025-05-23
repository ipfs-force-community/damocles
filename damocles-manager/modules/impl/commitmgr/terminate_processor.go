package commitmgr

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type TerminateProcessor struct {
	api       SealingAPI
	msgClient messager.API

	smgr core.SectorStateManager

	config *modules.SafeConfig

	prover core.Prover
}

func (tp TerminateProcessor) Process(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "terminate", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer plog.Infof("finished process, elapsed %s", time.Since(start))

	defer func() {
		for i := range sectors {
			if sectors[i].TerminateInfo.TerminateCid != nil {
				err := tp.smgr.Update(ctx, sectors[i].ID, core.WorkerOffline, sectors[i].TerminateInfo)
				if err != nil {
					plog.With("sector", sectors[i].ID.Number).Errorf("Update sector TerminateInfo failed: %s", err)
				}
			}
		}
	}()

	tok, _, err := tp.api.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("get chain head: %w", err)
	}
	nv, err := tp.api.StateNetworkVersion(ctx, tok)
	if err != nil {
		return fmt.Errorf("get network version : %w", err)
	}
	declMax, err := specpolicy.GetDeclarationsMax(nv)
	if err != nil {
		return fmt.Errorf("get max declarations: %w", err)
	}

	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return fmt.Errorf("actor id: %w", err)
	}

	dl, err := tp.api.StateMinerProvingDeadline(ctx, maddr, nil)
	if err != nil {
		return fmt.Errorf("getting proving deadline info: %w", err)
	}

	todo := map[miner.SectorLocation]*bitfield.BitField{}
	for _, sector := range sectors {
		loc, err := tp.api.StateSectorPartition(ctx, maddr, sector.ID.Number, nil)
		if err != nil {
			plog.Errorf("getting sector %d location: %s", sector.ID.Number, err)
			continue
		}
		if loc == nil {
			plog.Errorf("sector %d location not found", sector.ID.Number)
			continue
		}

		bf, ok := todo[*loc]
		if !ok {
			n := bitfield.New()
			bf = &n
			todo[*loc] = bf
		}
		bf.Set(uint64(sector.ID.Number))
	}

	params := core.TerminateSectorsParams{}
	var total uint64
	for loc, bfSectors := range todo {
		n, err := bfSectors.Count()
		if err != nil {
			plog.Error(
				"failed to count sectors to terminate",
				"deadline",
				loc.Deadline,
				"partition",
				loc.Partition,
				"error",
				err,
			)
			continue
		}

		// don't send terminations for currently challenged sectors
		//revive:disable-next-line:line-length-limit
		if loc.Deadline == (dl.Index+1)%stminer.WPoStPeriodDeadlines || // not in next (in case the terminate message takes a while to get on chain)
			loc.Deadline == dl.Index || // not in current
			(loc.Deadline+1)%stminer.WPoStPeriodDeadlines == dl.Index { // not in previous
			continue
		}

		if n < 1 {
			plog.Warn("zero sectors in bucket", "deadline", loc.Deadline, "partition", loc.Partition)
			continue
		}

		toTerminate, err := bfSectors.Copy()
		if err != nil {
			plog.Warn("copy sectors bitfield", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		ps, err := tp.api.StateMinerPartitions(ctx, maddr, loc.Deadline, nil)
		if err != nil {
			plog.Warn("getting miner partitions", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		toTerminate, err = bitfield.IntersectBitField(ps[loc.Partition].LiveSectors, toTerminate)
		if err != nil {
			plog.Warn(
				"intersecting liveSectors and toTerminate bitfields",
				"deadline",
				loc.Deadline,
				"partition",
				loc.Partition,
				"error",
				err,
			)
			continue
		}

		if total+n > uint64(stminer.AddressedSectorsMax) {
			n = uint64(stminer.AddressedSectorsMax) - total

			toTerminate, err = toTerminate.Slice(0, n)
			if err != nil {
				plog.Warn(
					"slice toTerminate bitfield",
					"deadline",
					loc.Deadline,
					"partition",
					loc.Partition,
					"error",
					err,
				)
				continue
			}

			s, err := bitfield.SubtractBitField(*bfSectors, toTerminate)
			if err != nil {
				plog.Warn("sectors-toTerminate", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
				continue
			}
			*bfSectors = s
		}

		total += n

		params.Terminations = append(params.Terminations, core.TerminationDeclaration{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   toTerminate,
		})

		if total >= uint64(stminer.AddressedSectorsMax) {
			break
		}

		if len(params.Terminations) >= declMax {
			break
		}
	}

	if len(params.Terminations) == 0 {
		return nil // nothing to do
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize TerminateSectorsParams: %w", err)
	}

	mcfg := tp.config.MustMinerConfig(mid)
	mcid, err := pushMessage(
		ctx,
		ctrlAddr,
		mid,
		big.Zero(),
		stbuiltin.MethodsMiner.TerminateSectors,
		tp.msgClient,
		&mcfg.Commitment.Terminate.Batch.FeeConfig,
		enc.Bytes(),
		plog,
	)
	if err != nil {
		return fmt.Errorf("push aggregate terminate message failed: %w", err)
	}

	plog.Info("push terminate success, cid: ", mcid)

	for _, t := range params.Terminations {
		delete(todo, miner.SectorLocation{
			Deadline:  t.Deadline,
			Partition: t.Partition,
		})

		err := t.Sectors.ForEach(func(sn uint64) error {
			for idx := range sectors {
				if sectors[idx].ID.Number == abi.SectorNumber(sn) {
					sectors[idx].TerminateInfo.TerminateCid = &mcid
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("sectors foreach: %w", err)
		}
	}

	return nil
}

func (tp TerminateProcessor) Expire(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
) (map[abi.SectorID]struct{}, error) {
	maxWait := tp.config.MustMinerConfig(mid).Commitment.Terminate.Batch.MaxWait.Std()
	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := tp.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	expire := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		if h-s.TerminateInfo.AddedHeight > maxWaitHeight {
			expire[s.ID] = struct{}{}
		}
	}

	return expire, nil
}

func (tp TerminateProcessor) CheckAfter(mid abi.ActorID) *time.Timer {
	return time.NewTimer(tp.config.MustMinerConfig(mid).Commitment.Terminate.Batch.CheckInterval.Std())
}

func (tp TerminateProcessor) Threshold(mid abi.ActorID) int {
	return tp.config.MustMinerConfig(mid).Commitment.Terminate.Batch.Threshold
}

var _ Processor = (*TerminateProcessor)(nil)
