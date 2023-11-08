package sectors

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/filestore"
)

var ErrProxiedTypedIndexerUnableForUpdating = fmt.Errorf("proxied typed indexer is unable for updating")

var _ core.SectorTypedIndexer = (*proxiedTypeIndexer)(nil)

type proxiedTypeIndexer struct {
	indexType core.SectorIndexType
	client    *core.SealerCliAPIClient
}

func (p *proxiedTypeIndexer) Find(ctx context.Context, sid abi.SectorID) (core.SectorAccessStores, bool, error) {
	found, err := p.client.SectorIndexerFind(ctx, p.indexType, sid)
	if err != nil {
		return core.SectorAccessStores{}, false, fmt.Errorf("call rpc method SectorIndexerFind: %w", err)
	}

	return found.Instance, found.Found, nil
}

func (p *proxiedTypeIndexer) Update(_ context.Context, _ abi.SectorID, _ core.SectorAccessStores) error {
	return ErrProxiedTypedIndexerUnableForUpdating
}

func NewProxiedIndexer(client *core.SealerCliAPIClient, storeMgr filestore.Manager) (core.SectorIndexer, error) {
	return &proxiedIndexer{
		client:   client,
		storeMgr: storeMgr,
	}, nil
}

type proxiedIndexer struct {
	client   *core.SealerCliAPIClient
	storeMgr filestore.Manager
}

func (p *proxiedIndexer) Normal() core.SectorTypedIndexer {
	return &proxiedTypeIndexer{
		indexType: core.SectorIndexTypeNormal,
		client:    p.client,
	}
}

func (p *proxiedIndexer) Upgrade() core.SectorTypedIndexer {
	return &proxiedTypeIndexer{
		indexType: core.SectorIndexTypeUpgrade,
		client:    p.client,
	}
}

func (p *proxiedIndexer) StoreMgr() filestore.Manager {
	return p.storeMgr
}
