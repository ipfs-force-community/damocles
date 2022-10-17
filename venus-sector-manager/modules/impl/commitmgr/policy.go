package commitmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
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
		return 0, fmt.Errorf("unsupported network vrsion: %w", err)
	}

	mpcd, err := policy.GetMaxProveCommitDuration(av, state.SectorType)
	if err != nil {
		return 0, fmt.Errorf("getting max prove commit duration: %w", err)
	}

	expiration, err := pp.sectorEnd(ctx, tok, height, state, mcfg.Sector.LifetimeDays, miner.WPoStProvingPeriod)
	if err != nil {
		return 0, fmt.Errorf("calculate sector end: %w", err)
	}

	if minExpiration := state.Ticket.Epoch + policy.MaxPreCommitRandomnessLookback + mpcd + policy.MinSectorExpiration; expiration < minExpiration {
		expiration = minExpiration
	}

	// Assume: both precommit msg & commit msg land on chain as early as possible
	maxExpiration := height + policy.GetPreCommitChallengeDelay() + policy.GetMaxSectorExpirationExtension()
	if expiration > maxExpiration {
		expiration = maxExpiration
	}

	return expiration, nil
}

func (pp PreCommitProcessor) sectorEnd(ctx context.Context, tok core.TipSetToken, height abi.ChainEpoch, state *core.SectorState, lifetimeDays uint64, provingPeriod abi.ChainEpoch) (abi.ChainEpoch, error) {
	var end *abi.ChainEpoch

	deals := state.Deals()

	for _, p := range deals {
		var endEpoch abi.ChainEpoch
		if p.Proposal == nil {
			proposal, err := pp.api.StateMarketStorageDealProposal(ctx, p.ID, tok)
			if err != nil {
				return 0, fmt.Errorf("get deal proposal for %d: %w", p.ID, err)
			}

			endEpoch = proposal.EndEpoch

		} else {
			endEpoch = p.Proposal.EndEpoch
		}

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
		expirationDuration := pp.ccSectorLifetime(abi.ChainEpoch(lifetimeDays*policy.EpochsInDay), provingPeriod)

		tmp := height + expirationDuration
		end = &tmp
	}

	// Ensure there is at least one day for the PC message to land without falling below min sector lifetime
	// TODO: The "one day" should probably be a config, though it doesn't matter too much
	minExp := height + policy.GetMinSectorExpiration() + provingPeriod
	if *end < minExp {
		end = &minExp
	}

	return *end, nil
}

func (pp PreCommitProcessor) ccSectorLifetime(ccLifetimeEpochs abi.ChainEpoch, provingPeriod abi.ChainEpoch) abi.ChainEpoch {
	if ccLifetimeEpochs == 0 {
		ccLifetimeEpochs = policy.GetMaxSectorExpirationExtension()
	}

	if minExpiration := abi.ChainEpoch(policy.MinSectorExpiration); ccLifetimeEpochs < minExpiration {
		log.Warnf("value for CommittedCapacitySectorLiftime is too short, using default minimum (%d epochs)", minExpiration)
		return minExpiration
	}
	if maxExpiration := policy.GetMaxSectorExpirationExtension(); ccLifetimeEpochs > maxExpiration {
		log.Warnf("value for CommittedCapacitySectorLiftime is too long, using default maximum (%d epochs)", maxExpiration)
		return maxExpiration
	}

	return ccLifetimeEpochs - provingPeriod*2
}
