package commitmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/venus/pkg/specactors"
	"github.com/filecoin-project/venus/pkg/specactors/policy"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

func Expiration(ctx context.Context, api SealingAPI, ps api.Deals) (abi.ChainEpoch, error) {
	tok, epoch, err := api.ChainHead(ctx)
	if err != nil {
		return 0, err
	}

	var end *abi.ChainEpoch

	for _, p := range ps {
		proposal, err := api.StateMarketStorageDealProposal(ctx, p.ID, tok)
		if err != nil {
			return 0, err
		}

		if proposal.EndEpoch < epoch {
			log.Warnf("piece schedule %+v ended before current epoch %d", p, epoch)
			continue
		}

		if end == nil || *end < proposal.EndEpoch {
			tmp := proposal.EndEpoch
			end = &tmp
		}
	}

	// we will limit min expire outside
	if end == nil {
		tmp := epoch
		end = &tmp
	}

	return *end, nil
}

func preCommitParams(ctx context.Context, stateMgr SealingAPI, sector api.SectorState) (*miner.SectorPreCommitInfo, big.Int, api.TipSetToken, error) {
	tok, _, err := stateMgr.ChainHead(ctx)
	if err != nil {
		log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
		return nil, big.Zero(), nil, err
	}

	maddr, err := address.NewIDAddress(uint64(sector.ID.Miner))
	if err != nil {
		return nil, big.Zero(), nil, err
	}

	if err := checkPrecommit(ctx, maddr, sector, stateMgr); err != nil {
		switch err := err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommitting: api error, not proceeding: %s", err)
			return nil, big.Zero(), nil, fmt.Errorf("call api failed %w", err)
		case *ErrBadCommD: // TODO: Should this just back to packing? (not really needed since handlePreCommit1 will do that too)
			return nil, big.Zero(), nil, fmt.Errorf("bad CommD error: %w", err)
		case *ErrExpiredTicket:
			return nil, big.Zero(), nil, fmt.Errorf("ticket expired: %w", err)
		case *ErrBadTicket:
			return nil, big.Zero(), nil, fmt.Errorf("bad ticket: %w", err)
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.ID, err)
			return nil, big.Zero(), nil, fmt.Errorf("invalid deals: %w", err)
		case *ErrExpiredDeals:
			return nil, big.Zero(), nil, fmt.Errorf("sector deals expired: %w", err)
		case *ErrPrecommitOnChain:
			return nil, big.Zero(), nil, fmt.Errorf("precommit land on chain")
		case *ErrSectorNumberAllocated:
			log.Errorf("handlePreCommitFailed: sector number already allocated, not proceeding: %s", err)
			// TODO: check if the sector is committed (not sure how we'd end up here)
			return nil, big.Zero(), nil, ErrSectorAllocated
		default:
			return nil, big.Zero(), nil, fmt.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	expiration, err := Expiration(ctx, stateMgr, sector.Deals)
	if err != nil {
		return nil, big.Zero(), nil, fmt.Errorf("handlePreCommitting: failed to compute pre-commit expiry: %w", err)
	}

	nv, err := stateMgr.StateNetworkVersion(ctx, tok)
	if err != nil {
		return nil, big.Zero(), nil, fmt.Errorf("failed to get network version: %w", err)
	}

	msd := policy.GetMaxProveCommitDuration(specactors.VersionForNetwork(nv), sector.SectorType)
	// TODO: get costumer config
	if minExpiration := sector.Ticket.Epoch + policy.MaxPreCommitRandomnessLookback + msd + miner.MinSectorExpiration; expiration < minExpiration {
		expiration = minExpiration
	}

	params := &miner.SectorPreCommitInfo{
		Expiration:   expiration,
		SectorNumber: sector.ID.Number,
		SealProof:    sector.SectorType,

		SealedCID:     sector.Pre.CommR,
		SealRandEpoch: sector.Ticket.Epoch,
		DealIDs:       sector.DealIDs(),
	}

	// TODO: upgrade sector

	deposit, err := stateMgr.StateMinerPreCommitDepositForPower(ctx, maddr, *params, tok)
	if err != nil {
		return nil, big.Zero(), nil, fmt.Errorf("getting initial pledge collateral: %w", err)
	}

	return params, deposit, tok, nil
}

func getSectorCollateral(ctx context.Context, stateMgr SealingAPI, mid abi.ActorID, sn abi.SectorNumber, tok api.TipSetToken) (abi.TokenAmount, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return big.Zero(), fmt.Errorf("invalid miner actor id: %w", err)
	}

	pci, err := stateMgr.StateSectorPreCommitInfo(ctx, maddr, sn, tok)
	if err != nil {
		return big.Zero(), fmt.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return big.Zero(), fmt.Errorf("precommit info not found on chain")
	}

	collateral, err := stateMgr.StateMinerInitialPledgeCollateral(ctx, maddr, pci.Info, tok)
	if err != nil {
		return big.Zero(), fmt.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, pci.PreCommitDeposit)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	return collateral, nil
}
