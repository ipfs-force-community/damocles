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
	"github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	messager "github.com/filecoin-project/venus-messager/types"
	specactors "github.com/filecoin-project/venus/pkg/specactors/builtin/miner"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type PreCommitProcessor struct {
	api       SealingAPI
	msgClient venusMessager.IMessager

	ds api.SectorsDatastore

	config Cfg
}

func (p PreCommitProcessor) processIndividually(ctx context.Context, sectors []api.Sector, from, maddr address.Address) {
	var spec messager.MsgMeta
	p.config.Lock()
	spec.GasOverEstimation = p.config.CommitmentManager[maddr].PreCommitGasOverEstimation
	spec.MaxFeeCap = p.config.CommitmentManager[maddr].MaxPreCommitFeeCap
	p.config.Lock()

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			defer wg.Done()

			params, deposit, _, err := preCommitParams(ctx, p.api, sectors[idx])
			if err != nil {
				log.Error("get pre-commit params failed: ", err)
			}
			enc := new(bytes.Buffer)
			if err := params.MarshalCBOR(enc); err != nil {
				log.Error("serialize pre-commit sector parameters failed: ", err)
				return
			}

			mcid, err := pushMessage(ctx, from, maddr, deposit, specactors.Methods.ProveCommitSector, p.msgClient, spec, enc.Bytes())
			if err != nil {
				log.Error("push pre-commit single failed: ", err)
				return
			}
			log.Infof("precommit of sector %d sent cid: %s", sectors[idx].SectorID.Number, mcid)

			sectors[idx].PreCommitCid = &mcid
		}(i)
	}
	wg.Wait()
}

func (p PreCommitProcessor) Process(ctx context.Context, sectors []api.Sector, maddr address.Address) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	defer cleanSector(ctx, sectors, p.ds)

	from, err := getPreCommitControlAddress(maddr, p.config)
	if err != nil {
		return fmt.Errorf("get pro commit control address failed: %s", err)
	}

	if !p.EnableBatch(maddr) {
		p.processIndividually(ctx, sectors, from, maddr)
		return nil
	}
	infos := []api.PreCommitEntry{}
	failed := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		params, deposit, _, err := preCommitParams(ctx, p.api, s)
		if err != nil {
			log.Errorf("get precommit %s %d params failed: %s\n", s.SectorID.Miner, s.SectorID.Number, err)
			failed[s.SectorID] = struct{}{}
			continue
		}
		infos = append(infos, api.PreCommitEntry{
			Deposit: deposit,
			Pci:     params,
		})
	}
	params := miner.PreCommitSectorBatchParams{}

	deposit := big.Zero()
	for i := range infos {
		params.Sectors = append(params.Sectors, *infos[i].Pci)
		deposit = big.Add(deposit, infos[i].Deposit)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize PreCommitSectorBatchParams: %s", err.Error())
	}
	var spec messager.MsgMeta
	p.config.Lock()
	spec.GasOverEstimation = p.config.CommitmentManager[maddr].BatchProCommitGasOverEstimation
	spec.MaxFeeCap = p.config.CommitmentManager[maddr].MaxBatchProCommitFeeCap
	p.config.Unlock()

	ccid, err := pushMessage(ctx, from, maddr, deposit, builtin5.MethodsMiner.PreCommitSectorBatch,
		p.msgClient, spec, enc.Bytes())
	if err != nil {
		return fmt.Errorf("push batch precommit message failed: %s", err.Error())
	}
	for i := range sectors {
		if _, ok := failed[sectors[i].SectorID]; !ok {
			sectors[i].PreCommitCid = &ccid
		}
	}
	return nil
}

func (p PreCommitProcessor) Expire(ctx context.Context, sectors []api.Sector, maddr address.Address) (map[abi.SectorID]struct{}, error) {
	p.config.Lock()
	maxWait := p.config.CommitmentManager[maddr].PreCommitBatchMaxWait
	p.config.Lock()
	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := p.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	expire := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		if h-s.SeedEpoch > maxWaitHeight {
			expire[s.SectorID] = struct{}{}
		}
	}

	return expire, nil
}

func (p PreCommitProcessor) CheckAfter(maddr address.Address) *time.Timer {
	p.config.Lock()
	defer p.config.Unlock()
	return time.NewTimer(p.config.CommitmentManager[maddr].PreCommitCheckInterval)
}

func (p PreCommitProcessor) Threshold(maddr address.Address) int {
	p.config.Lock()
	defer p.config.Unlock()
	return p.config.CommitmentManager[maddr].PreCommitBatchThreshold
}

func (p PreCommitProcessor) EnableBatch(maddr address.Address) bool {
	p.config.Lock()
	defer p.config.Unlock()
	return p.config.CommitmentManager[maddr].EnableBatchPreCommit
}

var _ Processor = (*PreCommitProcessor)(nil)
