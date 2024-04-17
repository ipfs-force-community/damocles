package commitmgr

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type PreCommitProcessor struct {
	api  SealingAPI
	mapi core.MinerAPI

	msgClient messager.API

	smgr core.SectorStateManager

	config *modules.SafeConfig
}

func (p PreCommitProcessor) Process(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "pre", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer plog.Infof("finished process, elapsed %s", time.Since(start))
	defer updateSector(ctx, p.smgr, sectors, plog)

	// For precommits the only method to precommit sectors after nv21 is to use the new precommit_batch2 method

	mcfg, err := p.config.MinerConfig(mid)
	if err != nil {
		return fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	infos := []core.PreCommitEntry{}
	for i := range sectors {
		s := &sectors[i]
		params, deposit, _, err := p.preCommitInfo(ctx, s)
		if err != nil {
			plog.Errorf("get precommit params for %d failed: %s\n", s.ID.Number, err)
			continue
		}

		infos = append(infos, core.PreCommitEntry{
			Deposit:     deposit,
			Pcsp:        params,
			SectorState: s,
		})
	}

	if len(infos) == 0 {
		return fmt.Errorf("no available sector infos for pre commit ")
	}

	sendPrecommit := func(infos []core.PreCommitEntry) error {
		if len(infos) > 1 {
			nums := make([]abi.SectorNumber, len(infos))
			for i := range infos {
				nums[i] = infos[i].Pcsp.SectorNumber
			}
			plog.Infof("batch precommit for %v", nums)
		}
		params := core.PreCommitSectorBatchParams{}
		deposit := big.Zero()
		for i := range infos {
			if infos[i].SectorState.HasDDODeal() {
				infos[i].Pcsp.DealIDs = nil // will be passed later in the Commit message
			}
			params.Sectors = append(params.Sectors, *infos[i].Pcsp)
			if mcfg.Commitment.Pre.SendFund {
				deposit = big.Add(deposit, infos[i].Deposit)
			}
		}

		enc := new(bytes.Buffer)
		if err := params.MarshalCBOR(enc); err != nil {
			return fmt.Errorf("couldn't serialize PreCommitSectorBatchParams: %w", err)
		}

		var err error
		var ccid cid.Cid
		if len(infos) > 1 {
			ccid, err = pushMessage(ctx, ctrlAddr, mid, deposit, stbuiltin.MethodsMiner.PreCommitSectorBatch2,
				p.msgClient, &mcfg.Commitment.Pre.Batch.FeeConfig, enc.Bytes(), plog)
		} else {
			ccid, err = pushMessage(ctx, ctrlAddr, mid, deposit, stbuiltin.MethodsMiner.PreCommitSectorBatch2,
				p.msgClient, &mcfg.Commitment.Pre.FeeConfig, enc.Bytes(), plog)
		}
		if err != nil {
			return fmt.Errorf("push message failed: %w", err)
		}

		for i := range infos {
			infos[i].SectorState.MessageInfo.PreCommitCid = &ccid
		}
		return nil
	}

	if p.ShouldBatch(mid) || len(infos) == 1 {
		return sendPrecommit(infos)
	}

	// handle precommit individually
	for i := range infos {
		err := sendPrecommit([]core.PreCommitEntry{infos[i]})
		if err != nil {
			plog.Errorf("send precommit for %d: %s", infos[i].Pcsp.SectorNumber, err)
		}
	}

	return nil
}

func (p PreCommitProcessor) Expire(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
) (map[abi.SectorID]struct{}, error) {
	maxWait := p.config.MustMinerConfig(mid).Commitment.Pre.Batch.MaxWait.Std()
	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := p.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	expire := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		if h-s.Ticket.Epoch > maxWaitHeight {
			expire[s.ID] = struct{}{}
		}
	}

	return expire, nil
}

func (p PreCommitProcessor) CheckAfter(mid abi.ActorID) *time.Timer {
	return time.NewTimer(p.config.MustMinerConfig(mid).Commitment.Pre.Batch.CheckInterval.Std())
}

func (p PreCommitProcessor) Threshold(mid abi.ActorID) int {
	return p.config.MustMinerConfig(mid).Commitment.Pre.Batch.Threshold
}

func (p PreCommitProcessor) EnableBatch(mid abi.ActorID) bool {
	return !p.config.MustMinerConfig(mid).Commitment.Pre.Batch.BatchCommitAboveBaseFee.IsZero()
}

func (p PreCommitProcessor) ShouldBatch(mid abi.ActorID) bool {
	if !p.EnableBatch(mid) {
		return false
	}
	bLog := log.With("actor", mid, "type", "pre")

	basefee, err := func() (abi.TokenAmount, error) {
		ctx := context.Background()
		tok, _, err := p.api.ChainHead(ctx)
		if err != nil {
			return abi.NewTokenAmount(0), err
		}
		return p.api.ChainBaseFee(ctx, tok)
	}()
	if err != nil {
		log.Errorf("get basefee: %w", err)
		return false
	}

	bcfg := p.config.MustMinerConfig(mid).Commitment.Pre.Batch
	basefeeAbove := basefee.GreaterThanEqual(abi.TokenAmount(bcfg.BatchCommitAboveBaseFee))
	bLog.Debugf(
		"should batch(%t): basefee(%s), basefee above(%s)",
		basefeeAbove,
		modules.FIL(basefee).Short(),
		bcfg.BatchCommitAboveBaseFee.Short(),
	)

	return basefeeAbove
}

var _ Processor = (*PreCommitProcessor)(nil)
