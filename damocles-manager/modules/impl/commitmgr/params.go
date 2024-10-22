package commitmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	v1 "github.com/filecoin-project/venus/venus-shared/api/chain/v1"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

func (p PreCommitProcessor) preCommitInfo(
	ctx context.Context,
	sector *core.SectorState,
) (*miner.SectorPreCommitInfo, big.Int, core.TipSetToken, error) {
	stateMgr := p.api
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
		case *ErrAPI:
			log.Errorf("handlePreCommitting: api error, not proceeding: %s", err)
			return nil, big.Zero(), nil, fmt.Errorf("call api failed %w", err)
		case *ErrBadCommD:
			// TODO: Should this just back to packing? (not really needed since handlePreCommit1 will do that too)
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

	expiration, err := p.sectorExpiration(ctx, sector)
	if err != nil {
		return nil, big.Zero(), nil, fmt.Errorf("handlePreCommitting: failed to compute pre-commit expiry: %w", err)
	}

	params := &miner.SectorPreCommitInfo{
		Expiration:   expiration,
		SectorNumber: sector.ID.Number,
		SealProof:    sector.SectorType,

		SealedCID:     sector.Pre.CommR,
		SealRandEpoch: sector.Ticket.Epoch,
	}

	if len(sector.Pieces) > 0 || len(sector.LegacyPieces) > 0 {
		params.UnsealedCid = &sector.Pre.CommD
	}

	// TODO: upgrade sector

	deposit, err := stateMgr.StateMinerPreCommitDepositForPower(ctx, maddr, *params, tok)
	if err != nil {
		return nil, big.Zero(), nil, fmt.Errorf("getting initial pledge collateral: %w", err)
	}

	return params, deposit, tok, nil
}

func getSectorCollateral(
	ctx context.Context,
	stateMgr SealingAPI,
	chainAPI v1.FullNode,
	sector *core.SectorState,
	mid abi.ActorID,
	ts *types.TipSet,
	pieces []lminer.PieceActivationManifest,
) (abi.TokenAmount, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return big.Zero(), fmt.Errorf("invalid miner actor id: %w", err)
	}

	pci, err := stateMgr.StateSectorPreCommitInfo(ctx, maddr, sector.ID.Number, ts.Key().Bytes())
	if err != nil {
		return big.Zero(), fmt.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return big.Zero(), fmt.Errorf("precommit info not found on chain")
	}

	duration := pci.Info.Expiration - ts.Height()

	ssize, err := pci.Info.SealProof.SectorSize()
	if err != nil {
		return big.Zero(), fmt.Errorf("failed to resolve sector size for seal proof: %w", err)
	}

	var verifiedSize uint64
	for _, piece := range pieces {
		if piece.VerifiedAllocationKey != nil {
			verifiedSize += uint64(piece.Size)
		}
	}

	collateral, err := chainAPI.StateMinerInitialPledgeForSector(ctx, duration, ssize, verifiedSize, ts.Key())
	if err != nil {
		return big.Zero(), fmt.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, pci.PreCommitDeposit)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	log.Infow(
		"getSectorCollateral",
		"collateral",
		types.FIL(collateral),
		"sn",
		sector.ID,
		"precommit",
		types.FIL(pci.PreCommitDeposit),
		"pledge",
		types.FIL(collateral),
		"verifiedSize",
		verifiedSize,
	)

	return collateral, nil
}

func getSectorCollateralNiPoRep(
	ctx context.Context,
	stateMgr SealingAPI,
	mid abi.ActorID,
	p *core.SectorState,
	tok core.TipSetToken,
	expire abi.ChainEpoch,
) (abi.TokenAmount, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return big.Zero(), fmt.Errorf("invalid miner actor id: %w", err)
	}

	collateral, err := stateMgr.StateMinerInitialPledgeCollateral(ctx, maddr, miner.SectorPreCommitInfo{
		Expiration:   expire,
		SectorNumber: p.ID.Number,
		SealProof:    p.SectorType,

		SealedCID:     p.Pre.CommR,
		SealRandEpoch: p.Ticket.Epoch,
	}, tok)
	if err != nil {
		return big.Zero(), fmt.Errorf("getting initial pledge collateral: %w", err)
	}
	return collateral, nil
}

func getProvingDeadline(
	ctx context.Context,
	stateMgr SealingAPI,
	mid abi.ActorID,
	tok core.TipSetToken,
) (uint64, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return 0, fmt.Errorf("invalid miner actor id: %w", err)
	}

	info, err := stateMgr.StateMinerProvingDeadline(ctx, maddr, tok)
	if err != nil {
		return 0, fmt.Errorf("getting miner proving deadline: %w", err)
	}
	return info.Index, nil
}
