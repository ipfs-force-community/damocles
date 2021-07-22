package commitmgr

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	messager "github.com/filecoin-project/venus-messager/types"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type CommitProcesser struct{}

func (c CommitProcesser) processIndividually(ctx context.Context, msgClient venusMessager.IMessager, sectors []api.Sector,
	api SealingAPI, from, maddr address.Address, config Cfg) {

	var spec messager.MsgMeta
	config.Lock()
	spec.GasOverEstimation = config.CommitmentManager.ProCommitGasOverEstimation
	spec.MaxFeeCap = config.CommitmentManager.MaxProCommitFeeCap
	config.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			defer wg.Done()
			err := pushCommitSingle(ctx, msgClient, from, maddr, &sectors[idx], spec, api)
			if err != nil {
				log.Error("push commit single failed: ", err)
			}
		}(i)
	}
	wg.Wait()
}

func (c CommitProcesser) Process(ctx context.Context, msgClient venusMessager.IMessager, sectors []api.Sector,
	enableBatch bool, sapi SealingAPI, wg *sync.WaitGroup, maddr address.Address, prover *api.Prover, ds api.SectorsDatastore, config Cfg) {
	defer func() {
		// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
		cleanSector(ctx, sectors, ds)
		wg.Done()
	}()

	from, err := getProCommitControlAddress(maddr, config)
	if err != nil {
		log.Errorf("get pro commit control address failed: %s", err)
		return
	}
	if !enableBatch || len(sectors) < miner.MinAggregatedSectors {
		c.processIndividually(ctx, msgClient, sectors, sapi, from, maddr, config)
		return
	}

	tok, _, err := sapi.ChainHead(ctx)
	if err != nil {
		log.Error("get chain head failed: ", err)
		return
	}

	infos := []proof5.AggregateSealVerifyInfo{}
	sectorsMap := map[abi.SectorNumber]api.Sector{}
	failed := map[abi.SectorID]struct{}{}

	for i, p := range sectors {
		sectorsMap[p.SectorID.Number] = sectors[i]
		_, err := getSectorCollateral(ctx, sapi, maddr, p.SectorID.Number, tok)
		if err != nil {
			log.Errorf("get sector %s %d collateral failed: %s\n", maddr, p.SectorID.Number, err)
			failed[sectors[i].SectorID] = struct{}{}
			continue
		}
		infos = append(infos, proof5.AggregateSealVerifyInfo{
			Number:                p.SectorID.Number,
			Randomness:            p.TicketValue,
			InteractiveRandomness: p.SeedValue,
			SealedCID:             *p.CommR,
			UnsealedCID:           *p.CommD,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Number < infos[j].Number
	})

	params := &miner.ProveCommitAggregateParams{
		SectorNumbers: bitfield.New(),
	}

	actorID, err := address.IDFromAddress(maddr)
	if err != nil {
		log.Error("trans maddr to actorID failed: ", err)
		return
	}

	proofs := make([][]byte, 0)
	collateral := big.Zero()
	for i := range infos {
		sc, err := getSectorCollateral(ctx, sapi, maddr, infos[i].Number, tok)
		if err != nil {
			// just log and keep the train on
			log.Errorf("get collateral failed: %s", err.Error())
		}
		collateral = big.Add(collateral, sc)
		params.SectorNumbers.Set(uint64(infos[i].Number))

		proofs = append(proofs, sectorsMap[infos[i].Number].Proof)
	}

	params.AggregateProof, err = (*prover).AggregateSealProofs(proof5.AggregateSealVerifyProofAndInfos{
		Miner:          abi.ActorID(actorID),
		SealProof:      sectorsMap[infos[0].Number].SectorType,
		AggregateProof: abi.RegisteredAggregationProof_SnarkPackV1,
		Infos:          infos,
	}, proofs)

	if err != nil {
		log.Errorf("aggregate sector failed: %s", err.Error())
		return
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		log.Errorf("couldn't serialize ProveCommitAggregateParams: %s", err.Error())
		return
	}
	var spec messager.MsgMeta
	config.Lock()
	spec.GasOverEstimation = config.CommitmentManager.BatchProCommitGasOverEstimation
	spec.MaxFeeCap = config.CommitmentManager.MaxBatchProCommitFeeCap
	config.Unlock()
	ccid, err := pushMessage(ctx, from, maddr, collateral, builtin5.MethodsMiner.ProveCommitAggregate,
		msgClient, spec, enc.Bytes())
	if err != nil {
		log.Errorf("push aggregate prove message failed: %s", err.Error())
		return
	}

	for i := range sectors {
		if _, ok := failed[sectors[i].SectorID]; !ok {
			sectors[i].CommitCid = &ccid
		}
	}
}

func (c CommitProcesser) PickTimeOutSector(ctx context.Context, sectors *[]api.Sector, sapi SealingAPI, config Cfg) ([]api.Sector, error) {
	config.Lock()
	maxWait := config.CommitmentManager.CommitBatchMaxWait
	config.Lock()

	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := sapi.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	res, remain := []api.Sector{}, []api.Sector{}
	for _, s := range *sectors {
		if h-s.TicketEpoch > maxWaitHeight {
			res = append(res, s)
		} else {
			remain = append(remain, s)
		}
	}
	sectors = &remain
	return res, nil
}

func (c CommitProcesser) NewTimer(config Cfg) *time.Timer {
	config.Lock()
	defer config.Unlock()
	return time.NewTimer(config.CommitmentManager.CommitCheckInterval)
}

func (c CommitProcesser) Threshold(config Cfg) int {
	config.Lock()
	defer config.Unlock()
	return config.CommitmentManager.CommitBatchThreshold
}

func (c CommitProcesser) EnableBatch(config Cfg) bool {
	config.Lock()
	defer config.Unlock()
	return config.CommitmentManager.EnableBatchProCommit
}

func cleanSector(ctx context.Context, sector []api.Sector, ds api.SectorsDatastore) {
	for i := range sector {
		sector[i].NeedSend = false
		err := ds.PutSector(ctx, sector[i])
		if err != nil {
			log.Error("clean sector failed: ", err)
		}
	}
}

var _ Processer = (*CommitProcesser)(nil)
