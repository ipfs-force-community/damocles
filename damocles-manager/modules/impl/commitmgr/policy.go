package commitmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
)

func (pp PreCommitProcessor) sectorExpiration(ctx context.Context, state *core.SectorState) (abi.ChainEpoch, error) {
	mcfg, err := pp.config.MinerConfig(state.ID.Miner)
	if err != nil {
		return 0, fmt.Errorf("get miner config: %w", err)
	}

	tok, height, err := pp.api.ChainHead(ctx)
	if err != nil {
		return 0, fmt.Errorf("get chain head: %w", err)
	}

	nv, err := pp.api.StateNetworkVersion(ctx, tok)
	if err != nil {
		return 0, fmt.Errorf("failed to get network version: %w", err)
	}

	av, err := actors.VersionForNetwork(nv)
	if err != nil {
		return 0, fmt.Errorf("unsupported network version: %w", err)
	}

	mpcd, err := policy.GetMaxProveCommitDuration(av, state.SectorType)
	if err != nil {
		return 0, fmt.Errorf("getting max prove commit duration: %w", err)
	}

	expiration, err := pp.sectorEnd(height, nv, state, mcfg.Sector.LifetimeDays, miner.WPoStProvingPeriod)
	if err != nil {
		return 0, fmt.Errorf("calculate sector end: %w", err)
	}

	if minExpiration := state.Ticket.Epoch + policy.MaxPreCommitRandomnessLookback + mpcd + policy.MinSectorExpiration; expiration < minExpiration {
		expiration = minExpiration
	}

	// Assume: both precommit msg & commit msg land on chain as early as possible
	maxLifetime, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return 0, fmt.Errorf("get max sector expiration extension: %w", err)
	}
	maxExpiration := height + policy.GetPreCommitChallengeDelay() + maxLifetime
	if expiration > maxExpiration {
		expiration = maxExpiration
	}

	return expiration, nil
}

func (pp PreCommitProcessor) sectorEnd(height abi.ChainEpoch, nv network.Version, state *core.SectorState, lifetimeDays uint64, provingPeriod abi.ChainEpoch) (abi.ChainEpoch, error) {
	var end *abi.ChainEpoch

	for _, p := range state.SectorPiece() {
		if !p.HasDealInfo() {
			continue
		}

		endEpoch := p.EndEpoch()
		if endEpoch < height {
			log.Warnf("piece schedule %+v ended before current epoch %d", p, height)
			continue
		}

		if end == nil || *end < endEpoch {
			tmp := endEpoch
			end = &tmp
		}
	}

	if end == nil {
		// no deal pieces, get expiration for committed capacity sector
		expirationDuration, err := pp.ccSectorLifetime(nv, abi.ChainEpoch(lifetimeDays*policy.EpochsInDay), provingPeriod)
		if err != nil {
			return 0, fmt.Errorf("failed to get cc sector lifetime: %w", err)
		}

		tmp := height + expirationDuration
		end = &tmp
	}

	// Ensure there is at least one day for the PC message to land without falling below min sector lifetime
	// TODO: The "one day" should probably be a config, though it doesn't matter too much
	minExp := height + policy.GetMinSectorExpiration() + miner.WPoStProvingPeriod
	if *end < minExp {
		end = &minExp
	}

	return *end, nil
}

func (pp PreCommitProcessor) ccSectorLifetime(nv network.Version, ccLifetimeEpochs abi.ChainEpoch, provingPeriod abi.ChainEpoch) (abi.ChainEpoch, error) {
	maxExpiration, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return 0, fmt.Errorf("get max sector expiration extension: %w", err)
	}

	if ccLifetimeEpochs == 0 {
		ccLifetimeEpochs = maxExpiration
	}

	if minExpiration := abi.ChainEpoch(policy.MinSectorExpiration); ccLifetimeEpochs < minExpiration {
		log.Warnf("value for CommittedCapacitySectorLifetime is too short, using default minimum (%d epochs)", minExpiration)
		return minExpiration, nil
	}
	if ccLifetimeEpochs > maxExpiration {
		log.Warnf("value for CommittedCapacitySectorLifetime is too long, using default maximum (%d epochs)", maxExpiration)
		return maxExpiration, nil
	}

	return ccLifetimeEpochs - provingPeriod*2, nil
}
