package dealmgr

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/hashicorp/go-multierror"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
)

var _ core.DealManager = (*DealManager)(nil)

func New(marketAPI market.API, infoAPI core.MinerInfoAPI, scfg *modules.SafeConfig, capi chain.API) *DealManager {
	return &DealManager{
		market: marketAPI,
		info:   infoAPI,
		scfg:   scfg,
		capi:   capi,
	}
}

type DealManager struct {
	market market.API
	info   core.MinerInfoAPI
	scfg   *modules.SafeConfig

	capi chain.API

	acquireMu sync.Mutex
}

func (dm *DealManager) Acquire(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec, lifetime *core.AcquireDealsLifetime, job core.SectorWorkerJob) (core.Deals, error) {
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

	minfo, err := dm.info.Get(ctx, sid.Miner)
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
	} else if mcfg.Sealing.SealingEpochDuration != 0 {
		h, err := dm.capi.ChainHead(ctx)
		if err != nil {
			return nil, fmt.Errorf("get chain head: %w", err)
		}
		mspec.StartEpoch = h.Height() + abi.ChainEpoch(mcfg.Sealing.SealingEpochDuration)
		mspec.EndEpoch = 1<<63 - 1
	}

	dm.acquireMu.Lock()
	defer dm.acquireMu.Unlock()

	dinfos, err := dm.market.AssignUnPackedDeals(ctx, sid, minfo.SectorSize, mspec)
	if err != nil {
		return nil, fmt.Errorf("assign non-packed deals: %w", err)
	}

	deals := make(core.Deals, 0, len(dinfos))
	for di := range dinfos {
		dinfo := dinfos[di]
		var proposal *core.DealProposal
		if dinfo.DealID != 0 {
			proposal = &dinfo.DealProposal
		}

		deals = append(deals, core.DealInfo{
			ID:          dinfo.DealID,
			PayloadSize: dinfo.PayloadSize,
			Piece: core.PieceInfo{
				Cid:  dinfo.PieceCID,
				Size: dinfo.PieceSize,
			},
			Proposal: proposal,
		})
	}

	return deals, nil
}

func (dm *DealManager) Release(ctx context.Context, sid abi.SectorID, deals core.Deals) error {
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
