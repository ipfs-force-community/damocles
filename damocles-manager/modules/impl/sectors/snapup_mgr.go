package sectors

import (
	"context"
	"fmt"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

var snapupLog = logging.New("sector-snapup")

var _ core.SnapUpSectorManager = (*SnapUpMgr)(nil)

//revive:disable-next-line:argument-limit
func NewSnapUpMgr(
	ctx context.Context,
	tracker core.SectorTracker,
	indexer core.SectorIndexer,
	chainAPI chain.API,
	eventbus *chain.EventBus,
	messagerAPI messager.API,
	minerAPI core.MinerAPI,
	stateMgr core.SectorStateManager,
	scfg *modules.SafeConfig,
	allocKVStore kvstore.KVStore,
	lookupID core.LookupID,
	senderSelector core.SenderSelector,
) (*SnapUpMgr, error) {
	allocator, err := NewSnapUpAllocator(chainAPI, minerAPI, allocKVStore, indexer, scfg)
	if err != nil {
		return nil, fmt.Errorf("construct snapup allocator: %w", err)
	}

	committer, err := NewSnapUpCommitter(
		ctx,
		tracker,
		indexer,
		chainAPI,
		eventbus,
		messagerAPI,
		stateMgr,
		scfg,
		lookupID,
		senderSelector,
	)
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
