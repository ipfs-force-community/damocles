package sectors

import (
	"context"
	"fmt"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

var snapupLog = logging.New("sector-snapup")

var _ core.SnapUpSectorManager = (*SnapUpMgr)(nil)

func NewSnapUpMgr(
	ctx context.Context,
	tracker core.SectorTracker,
	indexer core.SectorIndexer,
	chainAPI chain.API,
	eventbus *chain.EventBus,
	messagerAPI messager.API,
	minerInfoAPI core.MinerInfoAPI,
	stateMgr core.SectorStateManager,
	scfg *modules.SafeConfig,
	allocKVStore kvstore.KVStore,
) (*SnapUpMgr, error) {
	allocator, err := NewSnapUpAllocator(chainAPI, minerInfoAPI, allocKVStore, indexer, scfg)
	if err != nil {
		return nil, fmt.Errorf("construct snapup allocator: %w", err)
	}

	committer, err := NewSnapUpCommitter(ctx, tracker, indexer, chainAPI, eventbus, messagerAPI, stateMgr, scfg)
	if err != nil {
		return nil, fmt.Errorf("construct snapup committer: %w", err)
	}

	return &SnapUpMgr{
		SnapUpAllocator: allocator,
		SnapUpCommitter: committer,
	}, nil
}

type SnapUpMgr struct {
	*SnapUpAllocator
	*SnapUpCommitter
}
