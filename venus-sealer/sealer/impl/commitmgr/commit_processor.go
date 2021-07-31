package commitmgr

import (
	"bytes"
	"context"
	"fmt"
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
	specactors "github.com/filecoin-project/venus/pkg/specactors/builtin/miner"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type CommitProcessor struct {
	api       SealingAPI
	msgClient venusMessager.IMessager

	smgr api.SectorStateManager

	config Cfg

	prover api.Prover
}

func (c CommitProcessor) processIndividually(ctx context.Context, sectors []api.SectorState, from, maddr address.Address) {
	var spec messager.MsgMeta
	c.config.Lock()
	spec.GasOverEstimation = c.config.CommitmentManager[maddr].ProCommitGasOverEstimation
	spec.MaxFeeCap = c.config.CommitmentManager[maddr].MaxProCommitFeeCap
	c.config.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			defer wg.Done()

			params := &miner.ProveCommitSectorParams{
				SectorNumber: sectors[idx].ID.Number,
				Proof:        sectors[idx].Proof.Proof,
			}

			enc := new(bytes.Buffer)
			if err := params.MarshalCBOR(enc); err != nil {
				log.Error("serialize commit sector parameters failed: ", err)
				return
			}

			tok, _, err := c.api.ChainHead(ctx)
			if err != nil {
				log.Error("get chain head: ", err)
				return
			}

			collateral, err := getSectorCollateral(ctx, c.api, maddr, sectors[idx].ID.Number, tok)
			if err != nil {
				log.Error("get sector collateral failed: ", err)
				return
			}

			mcid, err := pushMessage(ctx, from, maddr, collateral, specactors.Methods.ProveCommitSector, c.msgClient, spec, enc.Bytes())
			if err != nil {
				log.Error("push commit single failed: ", err)
				return
			}

			log.Infof("prove of sector %d sent cid: %s", sectors[idx].ID.Number, mcid)
			sectors[idx].MessageInfo.CommitCid = &mcid
		}(i)
	}
	wg.Wait()
}

func (c CommitProcessor) Process(ctx context.Context, sectors []api.SectorState, maddr address.Address) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	defer c.cleanSector(ctx, sectors)

	from, err := getProCommitControlAddress(maddr, c.config)
	if err != nil {
		return fmt.Errorf("get pro commit control address failed: %w", err)
	}
	if !c.EnableBatch(maddr) || len(sectors) < miner.MinAggregatedSectors {
		c.processIndividually(ctx, sectors, from, maddr)
		return nil
	}

	tok, _, err := c.api.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("get chain head failed: %w", err)
	}

	infos := []proof5.AggregateSealVerifyInfo{}
	sectorsMap := map[abi.SectorNumber]api.SectorState{}
	failed := map[abi.SectorID]struct{}{}

	for i, p := range sectors {
		sectorsMap[p.ID.Number] = sectors[i]
		_, err := getSectorCollateral(ctx, c.api, maddr, p.ID.Number, tok)
		if err != nil {
			log.Errorf("get sector %s %d collateral failed: %s\n", maddr, p.ID.Number, err.Error())
			failed[sectors[i].ID] = struct{}{}
			continue
		}

		infos = append(infos, proof5.AggregateSealVerifyInfo{
			Number:                p.ID.Number,
			Randomness:            abi.SealRandomness(p.Ticket.Ticket),
			InteractiveRandomness: abi.InteractiveSealRandomness(p.Seed.Seed),
			SealedCID:             p.Pre.CommR,
			UnsealedCID:           p.Pre.CommD,
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
		return fmt.Errorf("trans maddr to actorID failed: %w", err)
	}

	proofs := make([][]byte, 0)
	collateral := big.Zero()
	for i := range infos {
		sc, err := getSectorCollateral(ctx, c.api, maddr, infos[i].Number, tok)
		if err != nil {
			// just log and keep the train on
			log.Errorf("get collateral failed: %s", err.Error())
		}
		collateral = big.Add(collateral, sc)
		params.SectorNumbers.Set(uint64(infos[i].Number))

		proofs = append(proofs, sectorsMap[infos[i].Number].Proof.Proof)
	}

	params.AggregateProof, err = c.prover.AggregateSealProofs(proof5.AggregateSealVerifyProofAndInfos{
		Miner:          abi.ActorID(actorID),
		SealProof:      sectorsMap[infos[0].Number].SectorType,
		AggregateProof: abi.RegisteredAggregationProof_SnarkPackV1,
		Infos:          infos,
	}, proofs)

	if err != nil {
		return fmt.Errorf("aggregate sector failed: %w", err)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize ProveCommitAggregateParams: %w", err)
	}
	var spec messager.MsgMeta
	c.config.Lock()
	spec.GasOverEstimation = c.config.CommitmentManager[maddr].BatchProCommitGasOverEstimation
	spec.MaxFeeCap = c.config.CommitmentManager[maddr].MaxBatchProCommitFeeCap
	c.config.Unlock()
	ccid, err := pushMessage(ctx, from, maddr, collateral, builtin5.MethodsMiner.ProveCommitAggregate,
		c.msgClient, spec, enc.Bytes())
	if err != nil {
		return fmt.Errorf("push aggregate prove message failed: %w", err)
	}

	for i := range sectors {
		if _, ok := failed[sectors[i].ID]; !ok {
			sectors[i].MessageInfo.CommitCid = &ccid
		}
	}

	return nil
}

func (c CommitProcessor) Expire(ctx context.Context, sectors []api.SectorState, maddr address.Address) (map[abi.SectorID]struct{}, error) {
	c.config.Lock()
	maxWait := c.config.CommitmentManager[maddr].CommitBatchMaxWait
	c.config.Lock()

	maxWaitHeight := abi.ChainEpoch(maxWait / (builtin.EpochDurationSeconds * time.Second))
	_, h, err := c.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	expire := map[abi.SectorID]struct{}{}
	for _, s := range sectors {
		if h-s.Seed.Epoch > maxWaitHeight {
			expire[s.ID] = struct{}{}
		}
	}
	return expire, nil
}

func (c CommitProcessor) CheckAfter(maddr address.Address) *time.Timer {
	c.config.Lock()
	defer c.config.Unlock()
	return time.NewTimer(c.config.CommitmentManager[maddr].CommitCheckInterval)
}

func (c CommitProcessor) Threshold(maddr address.Address) int {
	c.config.Lock()
	defer c.config.Unlock()
	return c.config.CommitmentManager[maddr].CommitBatchThreshold
}

func (c CommitProcessor) EnableBatch(maddr address.Address) bool {
	c.config.Lock()
	defer c.config.Unlock()
	return c.config.CommitmentManager[maddr].EnableBatchProCommit
}

func (c CommitProcessor) cleanSector(ctx context.Context, sector []api.SectorState) {
	for i := range sector {
		sector[i].MessageInfo.NeedSend = false
		err := c.smgr.Update(ctx, sector[i].ID, &sector[i].MessageInfo)
		if err != nil {
			log.Errorf("Update sector %s MessageInfo failed: ", err)
		}
	}
}

var _ Processor = (*CommitProcessor)(nil)
