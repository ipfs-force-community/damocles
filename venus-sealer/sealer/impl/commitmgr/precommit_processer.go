package commitmgr

import (
	"bytes"
	"context"
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

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type PreCommitProcesser struct{}

func (p PreCommitProcesser) processIndividually(ctx context.Context, msgClient venusMessager.IMessager, sectors []api.Sector,
	api SealingAPI, from, maddr address.Address, config Cfg) {

	var spec messager.MsgMeta
	config.Lock()
	spec.GasOverEstimation = config.CommitmentManager.PreCommitGasOverEstimation
	spec.MaxFeeCap = config.CommitmentManager.MaxPreCommitFeeCap
	config.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			defer wg.Done()
			err := pushPreCommitSingle(ctx, msgClient, from, maddr, &sectors[idx], spec, api)
			if err != nil {
				log.Error("push commit single failed: ", err)
			}
		}(i)
	}
	wg.Wait()
}

func (p PreCommitProcesser) Process(ctx context.Context, msgClient venusMessager.IMessager, sectors []api.Sector,
	enableBatch bool, sapi SealingAPI, wg *sync.WaitGroup, maddr address.Address, prover *Prover, ds api.SectorsDatastore, config Cfg) {
	defer func() {
		// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
		cleanSector(ctx, sectors, ds)
		wg.Done()
	}()

	from, err := getPreCommitControlAddress(maddr, config)
	if err != nil {
		log.Errorf("get pro commit control address failed: %s", err)
		return
	}

	if !enableBatch {
		p.processIndividually(ctx, msgClient, sectors, sapi, from, maddr, config)
		return
	}
	infos := []api.PreCommitEntry{}
	failed := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		params, deposit, _, err := preCommitParams(ctx, sapi, s)
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
		log.Errorf("couldn't serialize PreCommitSectorBatchParams: %s", err.Error())
		return
	}
	var spec messager.MsgMeta
	config.Lock()
	spec.GasOverEstimation = config.CommitmentManager.BatchProCommitGasOverEstimation
	spec.MaxFeeCap = config.CommitmentManager.MaxBatchProCommitFeeCap
	config.Unlock()

	ccid, err := pushMessage(ctx, from, maddr, deposit, builtin5.MethodsMiner.PreCommitSectorBatch,
		msgClient, spec, enc.Bytes())
	if err != nil {
		log.Errorf("push batch precommit message failed: %s", err.Error())
		return
	}
	for i := range sectors {
		if _, ok := failed[sectors[i].SectorID]; !ok {
			sectors[i].PreCommitCid = &ccid
		}
	}
}

func (p PreCommitProcesser) PickTimeOutSector(ctx context.Context, sectors *[]api.Sector, sapi SealingAPI, config Cfg) ([]api.Sector, error) {
	config.Lock()
	maxWait := config.CommitmentManager.PreCommitBatchMaxWait
	config.Lock()
	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := sapi.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	res, remain := []api.Sector{}, []api.Sector{}
	for _, s := range *sectors {
		if h-s.SeedEpoch > maxWaitHeight {
			res = append(res, s)
		} else {
			remain = append(remain, s)
		}
	}

	sectors = &remain
	return res, nil
}

func (p PreCommitProcesser) NewTimer(config Cfg) *time.Timer {
	config.Lock()
	defer config.Unlock()
	return time.NewTimer(config.CommitmentManager.PreCommitCheckInterval)
}

func (p PreCommitProcesser) Threshold(config Cfg) int {
	config.Lock()
	defer config.Unlock()
	return config.CommitmentManager.PreCommitBatchThreshold
}

func (p PreCommitProcesser) EnableBatch(config Cfg) bool {
	config.Lock()
	defer config.Unlock()
	return config.CommitmentManager.EnableBatchPreCommit
}

var _ Processer = (*PreCommitProcesser)(nil)
