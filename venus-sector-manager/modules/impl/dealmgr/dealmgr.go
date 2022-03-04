package dealmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/hashicorp/go-multierror"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
)

var _ api.DealManager = (*DealManager)(nil)

func New(marketAPI market.API, infoAPI api.MinerInfoAPI, scfg *modules.SafeConfig) *DealManager {
	return &DealManager{
		market: marketAPI,
		info:   infoAPI,
		scfg:   scfg,
	}
}

type DealManager struct {
	market market.API
	info   api.MinerInfoAPI
	scfg   *modules.SafeConfig
}

func (dm *DealManager) Acquire(ctx context.Context, sid abi.SectorID, maxDeals *uint) (api.Deals, error) {
	mcfg, err := dm.scfg.MinerConfig(sid.Miner)
	if err != nil {
		return nil, fmt.Errorf("get miner config: %w", err)
	}

	if !mcfg.Deal.Enabled {
		return nil, nil
	}

	minfo, err := dm.info.Get(ctx, sid.Miner)
	if err != nil {
		return nil, fmt.Errorf("get miner info: %w", err)
	}

	spec := &market.GetDealSpec{}
	if maxDeals != nil {
		spec.MaxPiece = int(*maxDeals)
	}

	dinfos, err := dm.market.AssignUnPackedDeals(ctx, minfo.Addr, minfo.SectorSize, spec)
	if err != nil {
		return nil, fmt.Errorf("assign non-packed deals: %w", err)
	}

	deals := make(api.Deals, 0, len(dinfos))
	for di := range dinfos {
		dinfo := dinfos[di]
		deals = append(deals, api.DealInfo{
			ID:          dinfo.DealID,
			PayloadSize: uint64(dinfo.PayloadSize),
			Piece: api.PieceInfo{
				Cid:  dinfo.PieceCID,
				Size: dinfo.PieceSize,
			},
		})
	}

	return deals, nil
}

func (dm *DealManager) Release(ctx context.Context, sid abi.SectorID, deals api.Deals) error {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return fmt.Errorf("invalid miner id %d: %w", sid.Miner, err)
	}

	var wg multierror.Group
	for i := range deals {
		dealID := deals[i].ID
		if dealID == 0 {
			continue
		}

		wg.Go(func() error {
			return dm.market.UpdateDealStatus(ctx, maddr, dealID, market.DealStatusUndefine)
		})
	}

	err = wg.Wait().ErrorOrNil()
	if err != nil {
		return fmt.Errorf("get errors in some or all of the requests: %w", err)
	}

	return nil
}
