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
	specactors "github.com/filecoin-project/venus/pkg/specactors/builtin/miner"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/messager"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type CommitProcessor struct {
	api       SealingAPI
	msgClient messager.API

	smgr api.SectorStateManager

	config Cfg

	prover api.Prover
}

func (c CommitProcessor) processIndividually(ctx context.Context, sectors []api.SectorState, from address.Address, mid abi.ActorID) {
	var spec messager.MsgMeta
	policy := c.config.policy(mid)
	spec.GasOverEstimation = policy.ProCommitGasOverEstimation
	spec.MaxFeeCap = policy.MaxProCommitFeeCap.Std()

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

			collateral, err := getSectorCollateral(ctx, c.api, mid, sectors[idx].ID.Number, tok)
			if err != nil {
				log.Error("get sector collateral failed: ", err)
				return
			}

			mcid, err := pushMessage(ctx, from, mid, collateral, specactors.Methods.ProveCommitSector, c.msgClient, spec, enc.Bytes())
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

func (c CommitProcessor) Process(ctx context.Context, sectors []api.SectorState, mid abi.ActorID, ctrlAddr address.Address) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	defer c.cleanSector(ctx, sectors)

	if !c.EnableBatch(mid) || len(sectors) < miner.MinAggregatedSectors {
		c.processIndividually(ctx, sectors, ctrlAddr, mid)
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
		_, err := getSectorCollateral(ctx, c.api, mid, p.ID.Number, tok)
		if err != nil {
			log.Errorf("get sector %d %d collateral failed: %s\n", mid, p.ID.Number, err.Error())
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

	proofs := make([][]byte, 0)
	collateral := big.Zero()
	for i := range infos {
		sc, err := getSectorCollateral(ctx, c.api, mid, infos[i].Number, tok)
		if err != nil {
			// just log and keep the train on
			log.Errorf("get collateral failed: %s", err.Error())
		}
		collateral = big.Add(collateral, sc)
		params.SectorNumbers.Set(uint64(infos[i].Number))

		proofs = append(proofs, sectorsMap[infos[i].Number].Proof.Proof)
	}

	params.AggregateProof, err = c.prover.AggregateSealProofs(proof5.AggregateSealVerifyProofAndInfos{
		Miner:          mid,
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
	policy := c.config.policy(mid)
	spec.GasOverEstimation = policy.BatchProCommitGasOverEstimation
	spec.MaxFeeCap = policy.MaxBatchProCommitFeeCap.Std()

	ccid, err := pushMessage(ctx, ctrlAddr, mid, collateral, builtin5.MethodsMiner.ProveCommitAggregate,
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

func (c CommitProcessor) Expire(ctx context.Context, sectors []api.SectorState, mid abi.ActorID) (map[abi.SectorID]struct{}, error) {
	maxWait := c.config.policy(mid).CommitBatchMaxWait.Std()
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

func (c CommitProcessor) CheckAfter(mid abi.ActorID) *time.Timer {
	return time.NewTimer(c.config.policy(mid).CommitCheckInterval.Std())
}

func (c CommitProcessor) Threshold(mid abi.ActorID) int {
	return c.config.policy(mid).CommitBatchThreshold
}

func (c CommitProcessor) EnableBatch(mid abi.ActorID) bool {
	return c.config.policy(mid).EnableBatchProCommit
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
