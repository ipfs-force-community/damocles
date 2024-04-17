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
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	miner13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util/piece"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
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

func (c CommitProcessor) processIndividually(
	ctx context.Context,
	sectors []core.SectorState,
	from address.Address,
	mid abi.ActorID,
	plog *logging.ZapLogger,
) {
	mcfg, err := c.config.MinerConfig(mid)
	if err != nil {
		plog.Errorf("get miner config for %d: %s", mid, err)
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(len(sectors))
	for i := range sectors {
		go func(idx int) {
			slog := plog.With("sector", sectors[idx].ID.Number)

			defer wg.Done()

			params := &miner.ProveCommitSectorParams{
				SectorNumber: sectors[idx].ID.Number,
				Proof:        sectors[idx].Proof.Proof,
			}

			enc := new(bytes.Buffer)
			if err := params.MarshalCBOR(enc); err != nil {
				slog.Error("serialize commit sector parameters failed: ", err)
				return
			}

			tok, _, err := c.api.ChainHead(ctx)
			if err != nil {
				slog.Error("get chain head: ", err)
				return
			}

			collateral := big.Zero()
			if mcfg.Commitment.Prove.SendFund {
				collateral, err = getSectorCollateral(ctx, c.api, mid, sectors[idx].ID.Number, tok)
				if err != nil {
					slog.Error("get sector collateral failed: ", err)
					return
				}
			}

			mcid, err := pushMessage(
				ctx,
				from,
				mid,
				collateral,
				stbuiltin.MethodsMiner.ProveCommitSector,
				c.msgClient,
				&mcfg.Commitment.Prove.FeeConfig,
				enc.Bytes(),
				slog,
			)
			if err != nil {
				slog.Error("push commit single failed: ", err)
				return
			}

			sectors[idx].MessageInfo.CommitCid = &mcid
			slog.Info("push commit success, cid: ", mcid)
		}(i)
	}
	wg.Wait()
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
	for i := range sectors {
		if sectors[i].HasBuiltinMarketDeal() {
			builtinMarketSectors = append(builtinMarketSectors, sectors[i])
		} else {
			ddoSectors = append(ddoSectors, sectors[i])
		}
	}

	aggregate := c.ShouldBatch(mid) && len(sectors) >= core.MinAggregatedSectors
	if nv >= MinDDONetworkVersion {
		if err := c.ProcessV2(ctx, ddoSectors, mid, ctrlAddr, tok, nv, aggregate); err != nil {
			return err
		}
	}
	return c.ProcessV1(ctx, builtinMarketSectors, mid, ctrlAddr, tok, nv, aggregate)
}

func (c CommitProcessor) ProcessV1(
	ctx context.Context,
	sectors []core.SectorState,
	mid abi.ActorID,
	ctrlAddr address.Address,
	tok core.TipSetToken,
	nv network.Version,
	batch bool,
) error {
	// Notice: If a sector in sectors has been sent, it's cid failed should be changed already.
	plog := log.With("proc", "prove", "miner", mid, "ctrl", ctrlAddr.String(), "len", len(sectors))

	start := time.Now()
	defer plog.Infof("finished process, elapsed %s", time.Since(start))

	defer updateSector(ctx, c.smgr, sectors, plog)

	if !batch {
		c.processIndividually(ctx, sectors, ctrlAddr, mid, plog)
		return nil
	}

	mcfg, err := c.config.MinerConfig(mid)
	if err != nil {
		return fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	arp, err := c.aggregateProofType(nv)
	if err != nil {
		return fmt.Errorf("get aggregate proof type: %w", err)
	}

	infos := []core.AggregateSealVerifyInfo{}
	sectorsMap := map[abi.SectorNumber]core.SectorState{}
	failed := map[abi.SectorID]struct{}{}

	collateral := big.Zero()
	for i, p := range sectors {
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

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Number < infos[j].Number
	})

	params := &miner.ProveCommitAggregateParams{
		SectorNumbers: bitfield.New(),
	}

	proofs := make([][]byte, 0)
	for i := range infos {
		params.SectorNumbers.Set(uint64(infos[i].Number))

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

	ccid, err := pushMessage(ctx, ctrlAddr, mid, collateral, stbuiltin.MethodsMiner.ProveCommitAggregate,
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
