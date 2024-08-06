package commitmgr

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	miner13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util/piece"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type CommitProcessor struct {
	chain     chainapi.API
	api       SealingAPI
	msgClient messager.API
	lookupID  core.LookupID

	smgr core.SectorStateManager

	config *modules.SafeConfig

	prover core.Prover
}

func (c CommitProcessor) Process(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
) error {
	tok, _, err := c.api.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("get chain head failed: %w", err)
	}

	nv, err := c.api.StateNetworkVersion(ctx, tok)
	if err != nil {
		return fmt.Errorf("get network version: %w", err)
	}

	ddoSectors := make([]core.SectorState, 0)
	builtinMarketSectors := make([]core.SectorState, 0)
	niporepSectors := make([]core.SectorState, 0)
	for i := range sectors {
		if sectors[i].SectorType.IsNonInteractive() {
			niporepSectors = append(niporepSectors, sectors[i])
		} else if sectors[i].HasBuiltinMarketDeal() {
			builtinMarketSectors = append(builtinMarketSectors, sectors[i])
		} else {
			ddoSectors = append(ddoSectors, sectors[i])
		}
	}
	fmt.Printf("%d niPoRep sectors going to process \n", len(niporepSectors))
	aggregate := c.ShouldBatch(mid) && len(sectors) >= core.MinAggregatedSectors
	if len(niporepSectors) > 0 {
		if err := c.ProcessNiPoRep(ctx, niporepSectors, mid, ctrlAddr, tok, nv, aggregate); err != nil {
			return err
		}
	}
	return c.ProcessV2(ctx, append(ddoSectors, builtinMarketSectors...), mid, ctrlAddr, tok, nv, aggregate)
}

// processBatchV2 processes a batch of sectors after nv22. It will always send
// ProveCommitSectors3Params which may contain either individual proofs or an
// aggregate proof depending on SP condition and network conditions.
func (c CommitProcessor) ProcessV2(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
	tok core.TipSetToken,
	nv network.Version,
	aggregate bool,
) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "prove", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer plog.Infof("finished process, elapsed %s", time.Since(start))

	defer updateSector(ctx, c.smgr, sectors, plog)

	mcfg, err := c.config.MinerConfig(mid)
	if err != nil {
		return fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	params := miner.ProveCommitSectors3Params{
		RequireActivationSuccess:   mcfg.Sealing.RequireActivationSuccess,
		RequireNotificationSuccess: mcfg.Sealing.RequireNotificationSuccess,
	}

	infos := []core.AggregateSealVerifyInfo{}
	sectorsMap := map[abi.SectorNumber]core.SectorState{}
	failed := map[abi.SectorID]struct{}{}

	// sort sectors by number
	sort.Slice(sectors, func(i, j int) bool { return sectors[i].ID.Number < sectors[j].ID.Number })

	collateral := big.Zero()
	for i, p := range sectors {
		activationManifest, dealIDs, err := piece.ProcessPieces(ctx, &sectors[i], c.chain, c.lookupID)
		if err != nil {
			return err
		}
		if len(dealIDs) > 0 {
			// DealID Precommit
			continue
		}
		sectorsMap[p.ID.Number] = sectors[i]
		if mcfg.Commitment.Prove.SendFund {
			sc, err := getSectorCollateral(ctx, c.api, mid, p.ID.Number, tok)
			if err != nil {
				plog.Errorf("get sector collateral for %d failed: %s\n", p.ID.Number, err)
				failed[sectors[i].ID] = struct{}{}
				continue
			}

			collateral = big.Add(collateral, sc)
		}

		params.SectorActivations = append(params.SectorActivations, miner13.SectorActivationManifest{
			SectorNumber: p.ID.Number,
			Pieces:       activationManifest,
		})
		params.SectorProofs = append(params.SectorProofs, p.Proof.Proof)

		infos = append(infos, core.AggregateSealVerifyInfo{
			Number:                p.ID.Number,
			Randomness:            abi.SealRandomness(p.Ticket.Ticket),
			InteractiveRandomness: abi.InteractiveSealRandomness(p.Seed.Seed),
			SealedCID:             p.Pre.CommR,
			UnsealedCID:           p.Pre.CommD,
		})
	}

	if len(infos) == 0 {
		return nil
	}

	if aggregate {
		proofs := make([][]byte, 0)
		for i := range infos {
			proofs = append(proofs, sectorsMap[infos[i].Number].Proof.Proof)
		}
		params.SectorProofs = nil // can't be set when aggregating
		arp, err := c.aggregateProofType(nv)
		if err != nil {
			return fmt.Errorf("get aggregate proof type: %w", err)
		}
		params.AggregateProofType = &arp
		params.AggregateProof, err = c.prover.AggregateSealProofs(ctx, core.AggregateSealVerifyProofAndInfos{
			Miner:          mid,
			SealProof:      sectorsMap[infos[0].Number].SectorType,
			AggregateProof: arp,
			Infos:          infos,
		}, proofs)

		if err != nil {
			return fmt.Errorf("aggregate sector failed: %w", err)
		}
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize ProveCommitSectors3Params: %w", err)
	}

	ccid, err := pushMessage(ctx, ctrlAddr, mid, collateral, stbuiltin.MethodsMiner.ProveCommitSectors3,
		c.msgClient, &mcfg.Commitment.Prove.Batch.FeeConfig, enc.Bytes(), plog)
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

// func (c CommitProcessor) ProcessNiPoRep(
// 	ctx context.Context,
// 	sectors []core.SectorState,
// 	mid abi.ActorID,
// 	ctrlAddr address.Address,
// 	tok core.TipSetToken,
// 	nv network.Version,
// 	batch bool,
// ) error {
// 	fmt.Println("[ni] start commit process")
// 	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
// 	plog := log.With("proc", "prove", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

// 	start := time.Now()
// 	defer func() {
// 		plog.Infof("finished process ni-porep, elapsed %s", time.Since(start))
// 	}()

// 	defer updateSector(ctx, c.smgr, sectors, plog)

// 	mcfg, err := c.config.MinerConfig(mid)
// 	if err != nil {
// 		return fmt.Errorf("get miner config for %d: %w", mid, err)
// 	}

// 	arp, err := c.aggregateProofType(nv)
// 	if err != nil {
// 		return fmt.Errorf("get aggregate proof type: %w", err)
// 	}
// 	fmt.Printf("[ni] commit process - %v\n", arp)
// 	// sectorsMap := map[abi.SectorNumber]core.SectorState{}
// 	failed := map[abi.SectorID]struct{}{}
// 	deadline, err := getProvingDeadline(ctx, c.api, mid, tok)
// 	if err != nil {
// 		return fmt.Errorf("get miner proving deadline for %d: %w", mid, err)
// 	}
// 	// avoid to use current or next deadline
// 	deadline = (deadline + mcfg.Sealing.SealingSectorDeadlineDelayNi) % miner.WPoStPeriodDeadlines

// 	for i, p := range sectors {
// 		var collateral big.Int
// 		expire, err := c.sectorExpiration(ctx, &p)
// 		if err != nil {
// 			plog.Errorf("get sector expiration for %d failed: %s\n", p.ID.Number, err)
// 			failed[sectors[i].ID] = struct{}{}
// 			continue
// 		}

// 		if mcfg.Commitment.Prove.SendFund {
// 			collateral, err = getSectorCollateralNiPoRep(ctx, c.api, mid, &p, tok, expire)
// 			if err != nil {
// 				plog.Errorf("get sector collateral for %d failed: %s\n", p.ID.Number, err)
// 				failed[sectors[i].ID] = struct{}{}
// 				continue
// 			}
// 		}

// 		params := &miner14.ProveCommitSectorsNIParams{
// 			Sectors: []miner14.SectorNIActivationInfo{{
// 				SealingNumber: p.ID.Number,
// 				SealerID:      mid,
// 				SealedCID:     p.Pre.CommR,
// 				SectorNumber:  p.ID.Number,
// 				SealRandEpoch: p.Seed.Epoch,
// 				Expiration:    expire,
// 			}},
// 			AggregateProof:           p.Proof.Proof,
// 			SealProofType:            p.SectorType,
// 			AggregateProofType:       arp,
// 			ProvingDeadline:          deadline,
// 			RequireActivationSuccess: true,
// 		}
// 		enc := new(bytes.Buffer)
// 		if err := params.MarshalCBOR(enc); err != nil {
// 			return fmt.Errorf("couldn't serialize ProveCommitSectorsNIParams: %w", err)
// 		}

// 		ccid, err := pushMessage(ctx, ctrlAddr, mid, collateral, stbuiltin.MethodsMiner.ProveCommitSectorsNI,
// 			c.msgClient, &mcfg.Commitment.Prove.FeeConfig, enc.Bytes(), plog)
// 		if err != nil {
// 			return fmt.Errorf("[ni] push prove message failed: %w", err)
// 		}
// 		fmt.Printf("[ni] success push message: %s\n", ccid)
// 		p.MessageInfo.CommitCid = &ccid
// 	}
// 	return nil
// }

func (c CommitProcessor) ProcessNiPoRep(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
	tok core.TipSetToken,
	nv network.Version,
	batch bool,
) error {
	fmt.Println("[ni] start commit process")
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "prove", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer func() {
		plog.Infof("finished process ni-porep, elapsed %s", time.Since(start))
	}()

	defer updateSector(ctx, c.smgr, sectors, plog)

	mcfg, err := c.config.MinerConfig(mid)
	if err != nil {
		return fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	arp, err := c.aggregateProofType(nv)
	if err != nil {
		return fmt.Errorf("get aggregate proof type: %w", err)
	}
	fmt.Printf("[ni] commit process - aggregateProofType %v\n", arp)
	infos := []core.AggregateSealVerifyInfo{}
	sectorsMap := map[abi.SectorNumber]core.SectorState{}
	failed := map[abi.SectorID]struct{}{}
	actInfos := []miner14.SectorNIActivationInfo{}

	collateral := big.Zero()
	ir := [32]byte{
		1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1,
	}

	for i, p := range sectors {
		sectorsMap[p.ID.Number] = sectors[i]
		expire, err := c.sectorExpiration(ctx, &p)
		if err != nil {
			plog.Errorf("get sector expiration for %d failed: %s\n", p.ID.Number, err)
			failed[sectors[i].ID] = struct{}{}
			continue
		}

		if mcfg.Commitment.Prove.SendFund {
			sc, err := getSectorCollateralNiPoRep(ctx, c.api, mid, &p, tok, expire)
			if err != nil {
				plog.Errorf("get sector collateral for %d failed: %s\n", p.ID.Number, err)
				failed[sectors[i].ID] = struct{}{}
				continue
			}

			collateral = big.Add(collateral, sc)
		}

		infos = append(infos, core.AggregateSealVerifyInfo{
			Number:                p.ID.Number,
			Randomness:            abi.SealRandomness(p.Ticket.Ticket),
			InteractiveRandomness: ir[:],
			SealedCID:             p.Pre.CommR,
			UnsealedCID:           p.Pre.CommD,
		})

		actInfos = append(actInfos, miner14.SectorNIActivationInfo{
			SealingNumber: p.ID.Number,
			SealerID:      mid,
			SealedCID:     p.Pre.CommR,
			SectorNumber:  p.ID.Number,
			SealRandEpoch: p.Seed.Epoch,
			Expiration:    expire,
		})
	}
	fmt.Printf("[ni] commit process - collateral %d\n", collateral)
	if len(infos) == 0 {
		return nil
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Number < infos[j].Number
	})

	sort.Slice(actInfos, func(i, j int) bool {
		return actInfos[i].SealingNumber < actInfos[j].SealingNumber
	})
	fmt.Println("actInfos")
	fmt.Printf("%#v\n\n", actInfos)

	fmt.Println("infos")
	fmt.Printf("%#v\n", infos)
	deadline, err := getProvingDeadline(ctx, c.api, mid, tok)
	if err != nil {
		return fmt.Errorf("get miner proving deadline for %d: %w", mid, err)
	}

	// avoid to use current or next deadline
	deadline = (deadline + mcfg.Sealing.SealingSectorDeadlineDelayNi) % miner.WPoStPeriodDeadlines
	fmt.Printf("deadline %d\n", deadline)
	params := &miner14.ProveCommitSectorsNIParams{
		Sectors:                  actInfos,
		SealProofType:            sectorsMap[infos[0].Number].SectorType,
		AggregateProofType:       arp,
		ProvingDeadline:          deadline,
		RequireActivationSuccess: true,
	}
	fmt.Printf("[ni] commit process - prooftype: %d\n", params.SealProofType)
	proofs := make([][]byte, 0)
	for i := range infos {
		proofs = append(proofs, sectorsMap[infos[i].Number].Proof.Proof)
	}

	params.AggregateProof, err = c.prover.AggregateSealProofs(ctx, core.AggregateSealVerifyProofAndInfos{
		Miner:          mid,
		SealProof:      sectorsMap[infos[0].Number].SectorType,
		AggregateProof: arp,
		Infos:          infos,
	}, proofs)

	if err != nil {
		return fmt.Errorf("aggregate sector failed: %w", err)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("couldn't serialize ProveCommitAggregateParams: %w", err)
	}

	ccid, err := pushMessage2(ctx, ctrlAddr, mid, collateral, stbuiltin.MethodsMiner.ProveCommitSectorsNI,
		c.msgClient, &mcfg.Commitment.Prove.Batch.FeeConfig, enc.Bytes(), plog, c.chain)
	if err != nil {
		return fmt.Errorf("push aggregate prove message failed: %w", err)
	}
	fmt.Printf("success push message: %s\n", ccid)
	for i := range sectors {
		if _, ok := failed[sectors[i].ID]; !ok {
			sectors[i].MessageInfo.CommitCid = &ccid
		} else {
			fmt.Printf("[ni] failed sector id: %d\n", sectors[i].ID)
		}
	}

	return nil
}

func (CommitProcessor) aggregateProofType(nv network.Version) (abi.RegisteredAggregationProof, error) {
	if nv < network.Version16 {
		return abi.RegisteredAggregationProof_SnarkPackV1, nil
	}
	return abi.RegisteredAggregationProof_SnarkPackV2, nil
}

func (c CommitProcessor) Expire(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
) (map[abi.SectorID]struct{}, error) {
	maxWait := c.config.MustMinerConfig(mid).Commitment.Prove.Batch.MaxWait.Std()
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
	return time.NewTimer(c.config.MustMinerConfig(mid).Commitment.Prove.Batch.CheckInterval.Std())
}

func (c CommitProcessor) Threshold(mid abi.ActorID) int {
	return c.config.MustMinerConfig(mid).Commitment.Prove.Batch.Threshold
}

func (c CommitProcessor) EnableBatch(mid abi.ActorID) bool {
	return !c.config.MustMinerConfig(mid).Commitment.Prove.Batch.BatchCommitAboveBaseFee.IsZero()
}

func (c CommitProcessor) ShouldBatch(mid abi.ActorID) bool {
	if !c.EnableBatch(mid) {
		return false
	}

	bLog := log.With("actor", mid, "type", "prove")

	basefee, err := func() (abi.TokenAmount, error) {
		ctx := context.Background()
		tok, _, err := c.api.ChainHead(ctx)
		if err != nil {
			return abi.NewTokenAmount(0), err
		}
		return c.api.ChainBaseFee(ctx, tok)
	}()
	if err != nil {
		bLog.Errorf("get basefee: %w", err)
		return false
	}

	bcfg := c.config.MustMinerConfig(mid).Commitment.Prove.Batch
	basefeeAbove := basefee.GreaterThanEqual(abi.TokenAmount(bcfg.BatchCommitAboveBaseFee))
	bLog.Debugf(
		"should batch(%t): basefee(%s), basefee above(%s)",
		basefeeAbove,
		modules.FIL(basefee).Short(),
		bcfg.BatchCommitAboveBaseFee.Short(),
	)

	return basefeeAbove
}

var _ Processor = (*CommitProcessor)(nil)
