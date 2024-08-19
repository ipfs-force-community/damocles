package pledge

import (
	"context"
	"fmt"

	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus/venus-shared/actors/adt"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/power"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/reward"
	bstore "github.com/filecoin-project/venus/venus-shared/blockstore"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

var (
	initialPledgeNum = types.NewInt(110)
	initialPledgeDen = types.NewInt(100)
)

func CalcPledgeForPower(ctx context.Context, api chain.API, addedPower abi.StoragePower) (abi.TokenAmount, error) {
	store := adt.WrapStore(ctx, cbor.NewCborStore(bstore.NewAPIBlockstore(api)))

	// load power actor
	var (
		powerSmoothed    builtin.FilterEstimate
		pledgeCollateral abi.TokenAmount
	)
	//nolint:all
	if act, err := api.StateGetActor(ctx, power.Address, types.EmptyTSK); err != nil {
		return types.EmptyInt, fmt.Errorf("loading power actor: %w", err)
	} else if s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, fmt.Errorf("loading power actor state: %w", err)
	} else if p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, fmt.Errorf("failed to determine total power: %w", err)
	} else if c, err := s.TotalLocked(); err != nil {
		return types.EmptyInt, fmt.Errorf("failed to determine pledge collateral: %w", err)
	} else {
		powerSmoothed = p
		pledgeCollateral = c
	}

	// load reward actor
	rewardActor, err := api.StateGetActor(ctx, reward.Address, types.EmptyTSK)
	if err != nil {
		return types.EmptyInt, fmt.Errorf("loading reward actor: %w", err)
	}

	rewardState, err := reward.Load(store, rewardActor)
	if err != nil {
		return types.EmptyInt, fmt.Errorf("loading reward actor state: %w", err)
	}

	// get circulating supply
	circSupply, err := api.StateVMCirculatingSupplyInternal(ctx, types.EmptyTSK)
	if err != nil {
		return big.Zero(), fmt.Errorf("getting circulating supply: %w", err)
	}

	// do the calculation
	initialPledge, err := rewardState.InitialPledgeForPower(
		addedPower,
		pledgeCollateral,
		&powerSmoothed,
		circSupply.FilCirculating,
	)
	if err != nil {
		return big.Zero(), fmt.Errorf("calculating initial pledge: %w", err)
	}

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

func SectorWeight(
	ctx context.Context,
	sector *core.SectorState,
	proofType abi.RegisteredSealProof,
	chainAPI chain.API,
	expiration abi.ChainEpoch,
) (abi.StoragePower, error) {
	ssize, err := proofType.SectorSize()
	if err != nil {
		return types.EmptyInt, fmt.Errorf("getting sector size: %w", err)
	}

	ts, err := chainAPI.ChainHead(ctx)
	if err != nil {
		return types.EmptyInt, fmt.Errorf("getting chain head: %w", err)
	}

	// get verified deal infos
	w, vw := big.Zero(), big.Zero()
	sectorDuration := big.NewInt(int64(expiration - ts.Height()))
	for _, piece := range sector.SectorPiece() {
		if !piece.HasDealInfo() {
			// todo StateMinerInitialPledgeCollateral doesn't add cc/padding to non-verified weight, is that correct?
			continue
		}

		pieceInfo := piece.PieceInfo()

		alloc, err := GetAllocation(ctx, chainAPI, ts.Key(), piece)
		if err != nil || alloc == nil {
			w = big.Add(w, big.Mul(sectorDuration, abi.NewStoragePower(int64(pieceInfo.Size))))
			continue
		}

		vw = big.Add(vw, big.Mul(sectorDuration, abi.NewStoragePower(int64(pieceInfo.Size))))
	}

	// load market actor
	duration := expiration - ts.Height()
	sectorWeight := builtin.QAPowerForWeight(ssize, duration, w, vw)

	return sectorWeight, nil
}
