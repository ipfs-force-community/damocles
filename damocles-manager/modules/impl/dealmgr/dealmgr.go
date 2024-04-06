package dealmgr

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/market"
)

var _ core.DealManager = (*DealManager)(nil)

func New(marketAPI market.API, minerAPI core.MinerAPI, scfg *modules.SafeConfig) *DealManager {
	return &DealManager{
		market:   marketAPI,
		minerAPI: minerAPI,
		scfg:     scfg,
	}
}

type DealManager struct {
	market   market.API
	minerAPI core.MinerAPI
	scfg     *modules.SafeConfig

	acquireMu sync.Mutex
}

func (dm *DealManager) Acquire(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec, lifetime *core.AcquireDealsLifetime, job core.SectorWorkerJob) (core.SectorPieces, error) {
	mcfg, err := dm.scfg.MinerConfig(sid.Miner)
	if err != nil {
		return nil, fmt.Errorf("get miner config: %w", err)
	}

	enabled := false
	switch job {
	case core.SectorWorkerJobSealing:
		enabled = mcfg.Sector.EnableDeals

	case core.SectorWorkerJobSnapUp:
		enabled = mcfg.SnapUp.Enabled

	}

	if !enabled {
		return nil, nil
	}

	minfo, err := dm.minerAPI.GetInfo(ctx, sid.Miner)
	if err != nil {
		return nil, fmt.Errorf("get miner info: %w", err)
	}

	mspec := &market.GetDealSpec{}
	if spec.MaxDeals != nil {
		mspec.MaxPiece = int(*spec.MaxDeals)
	}

	if spec.MinUsedSpace != nil {
		mspec.MinUsedSpace = *spec.MinUsedSpace
	}

	if lifetime != nil {
		mspec.StartEpoch = lifetime.Start
		mspec.EndEpoch = lifetime.End
		mspec.SectorExpiration = lifetime.SectorExpiration
	}

	dm.acquireMu.Lock()
	defer dm.acquireMu.Unlock()

	dinfos, err := dm.market.AssignDeals(ctx, sid, minfo.SectorSize, mspec)
	if err != nil {
		return nil, fmt.Errorf("assign non-packed deals: %w", err)
	}

	deals := make(core.SectorPieces, 0, len(dinfos))
	for di := range dinfos {
		dinfo := dinfos[di]
		deals = append(deals, core.SectorPieceV2{
			Piece: abi.PieceInfo{
				Size:     dinfo.PieceSize,
				PieceCID: dinfo.PieceCID,
			},
			DealInfo: &core.DealInfoV2{
				DealInfoV2:      dinfo,
				IsBuiltinMarket: dinfo.IsBuiltinMarket(),
			},
		})
	}

	return deals, nil
}

func (dm *DealManager) Release(ctx context.Context, sid abi.SectorID, deals core.SectorPieces) error {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return fmt.Errorf("invalid miner id %d: %w", sid.Miner, err)
	}

	builtinMarketDealIDs := make([]abi.DealID, 0)
	ddoAllocationIDs := make([]types.AllocationId, 0)

	for i := range deals {
		if !deals[i].HasDealInfo() {
			continue
		}

		if deals[i].IsBuiltinMarket() {
			builtinMarketDealIDs = append(builtinMarketDealIDs, deals[i].DealID())
		} else {
			ddoAllocationIDs = append(ddoAllocationIDs, deals[i].AllocationID())
		}
	}

	if len(builtinMarketDealIDs) > 0 {
		err = dm.market.ReleaseDeals(ctx, maddr, builtinMarketDealIDs)
		if err != nil {
			return fmt.Errorf("release builtin market deals: %w", err)
		}
	}

	if len(ddoAllocationIDs) > 0 {
		err = dm.market.ReleaseDirectDeals(ctx, maddr, ddoAllocationIDs)
		if err != nil {
			return fmt.Errorf("release ddo deals: %w", err)
		}
	}

	return nil
}

func (dm *DealManager) ReleaseLegacyDeal(ctx context.Context, sid abi.SectorID, deals core.Deals) error {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return fmt.Errorf("invalid miner id %d: %w", sid.Miner, err)
	}

	builtinMarketDealIDs := make([]abi.DealID, 0)

	for i := range deals {
		builtinMarketDealIDs = append(builtinMarketDealIDs, deals[i].ID)
	}

	if len(builtinMarketDealIDs) > 0 {
		err = dm.market.ReleaseDeals(ctx, maddr, builtinMarketDealIDs)
		if err != nil {
			return fmt.Errorf("release builtin market deals: %w", err)
		}
	}
	return nil
}
