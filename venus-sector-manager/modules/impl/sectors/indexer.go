package sectors

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var _ core.SectorIndexer = (*Indexer)(nil)

func NewIndexer(storeMgr objstore.Manager, normal kvstore.KVStore, upgrade kvstore.KVStore) (*Indexer, error) {
	return &Indexer{
		storeMgr: storeMgr,
		normal:   &innerIndexer{kv: normal},
		upgrade:  &innerIndexer{kv: upgrade},
	}, nil
}

type Indexer struct {
	normal   *innerIndexer
	upgrade  *innerIndexer
	storeMgr objstore.Manager
}

func (i *Indexer) Normal() core.SectorTypedIndexer {
	return i.normal
}

func (i *Indexer) Upgrade() core.SectorTypedIndexer {
	return i.upgrade
}

func (i *Indexer) StoreMgr() objstore.Manager {
	return i.storeMgr
}

type innerIndexer struct {
	kv kvstore.KVStore
}

func (i *innerIndexer) Find(ctx context.Context, sid abi.SectorID) (string, bool, error) {
	var s string
	// string(b) will copy the underlying bytes, so we use View here
	err := i.kv.View(ctx, makeSectorKey(sid), func(b []byte) error {
		s = string(b)
		return nil
	})

	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return "", false, nil
		}

		return "", false, err
	}

	return s, true, nil
}

func (i *innerIndexer) Update(ctx context.Context, sid abi.SectorID, instance string) error {
	return i.kv.Put(ctx, makeSectorKey(sid), []byte(instance))
}
