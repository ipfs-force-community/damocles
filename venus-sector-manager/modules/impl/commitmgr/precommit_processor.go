package commitmgr

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

type PreCommitProcessor struct {
	api  SealingAPI
	mapi core.MinerAPI

	msgClient messager.API

	smgr core.SectorStateManager

	config *modules.SafeConfig
}

func (p PreCommitProcessor) processIndividually(ctx context.Context, sectors []core.SectorState, from address.Address, mid abi.ActorID, l *logging.ZapLogger) {
	mcfg, err := p.config.MinerConfig(mid)
	if err != nil {
		l.Errorf("get miner config for %d: %s", mid, err)
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			slog := l.With("sector", sectors[idx].ID.Number)

			defer wg.Done()

			params, deposit, _, err := p.preCommitParams(ctx, sectors[idx])
			if err != nil {
				slog.Error("get pre-commit params failed: ", err)
				return
			}

			enc := new(bytes.Buffer)
			if err := params.MarshalCBOR(enc); err != nil {
				slog.Error("serialize pre-commit sector parameters failed: ", err)
				return
			}

			if !mcfg.Commitment.Pre.SendFund {
				deposit = big.Zero()
			}

			mcid, err := pushMessage(ctx, from, mid, deposit, stbuiltin.MethodsMiner.PreCommitSector, p.msgClient, &mcfg.Commitment.Pre.FeeConfig, enc.Bytes(), slog)
			if err != nil {
				slog.Error("push pre-commit single failed: ", err)
				return
			}

			sectors[idx].MessageInfo.PreCommitCid = &mcid
			slog.Info("push pre-commit success, cid: ", mcid)
		}(i)
	}
	wg.Wait()
}

func (p PreCommitProcessor) Process(ctx context.Context, sectors []core.SectorState, mid abi.ActorID, ctrlAddr address.Address) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "pre", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer plog.Infof("finished process, elapsed %s", time.Since(start))
	defer updateSector(ctx, p.smgr, sectors, plog)

	if !p.EnableBatch(mid) {
		p.processIndividually(ctx, sectors, ctrlAddr, mid, plog)
		return nil
	}

	mcfg, err := p.config.MinerConfig(mid)
	if err != nil {
		return fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	infos := []core.PreCommitEntry{}
	failed := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		params, deposit, _, err := p.preCommitParams(ctx, s)
		if err != nil {
			plog.Errorf("get precommit params for %d failed: %s\n", s.ID.Number, err)
			failed[s.ID] = struct{}{}
			continue
		}

		infos = append(infos, core.PreCommitEntry{
			Deposit: deposit,
			Pcsp:    params,
		})
	}

	if len(infos) == 0 {
		return fmt.Errorf("no available sector infos for pre commit batching")
	}

	params := core.PreCommitSectorBatchParams{}

	deposit := big.Zero()

	for i := range infos {
		params.Sectors = append(params.Sectors, *infos[i].Pcsp)
		if mcfg.Commitment.Pre.SendFund {
			deposit = big.Add(deposit, infos[i].Deposit)
		}
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize PreCommitSectorBatchParams: %w", err)
	}

	ccid, err := pushMessage(ctx, ctrlAddr, mid, deposit, stbuiltin.MethodsMiner.PreCommitSectorBatch,
		p.msgClient, &mcfg.Commitment.Pre.Batch.FeeConfig, enc.Bytes(), plog)
	if err != nil {
		return fmt.Errorf("push batch precommit message failed: %w", err)
	}
	for i := range sectors {
		if _, ok := failed[sectors[i].ID]; !ok {
			sectors[i].MessageInfo.PreCommitCid = &ccid
		}
	}
	return nil
}

func (p PreCommitProcessor) Expire(ctx context.Context, sectors []core.SectorState, mid abi.ActorID) (map[abi.SectorID]struct{}, error) {
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
	return p.config.MustMinerConfig(mid).Commitment.Pre.Batch.Enabled
}

var _ Processor = (*PreCommitProcessor)(nil)
