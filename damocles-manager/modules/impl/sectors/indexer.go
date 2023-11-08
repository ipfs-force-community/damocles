package sectors

import (
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/filestore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

var _ core.SectorIndexer = (*Indexer)(nil)

func NewIndexer(storeMgr filestore.Manager, normal kvstore.KVStore, upgrade kvstore.KVStore) (*Indexer, error) {
	return &Indexer{
		storeMgr: storeMgr,
		normal:   &innerIndexer{kv: normal},
		upgrade:  &innerIndexer{kv: upgrade},
	}, nil
}

type Indexer struct {
	normal   *innerIndexer
	upgrade  *innerIndexer
	storeMgr filestore.Manager
}

func (i *Indexer) Normal() core.SectorTypedIndexer {
	return i.normal
}

func (i *Indexer) Upgrade() core.SectorTypedIndexer {
	return i.upgrade
}

func (i *Indexer) StoreMgr() filestore.Manager {
	return i.storeMgr
}

func makeSectorKeySealedFile(sid abi.SectorID) kvstore.Key {
	return makeSectorKey(sid)
}

func makeSectorKeyForCacheDir(sid abi.SectorID) kvstore.Key {
	return []byte(fmt.Sprintf("cache/m-%d-n-%d", sid.Miner, sid.Number))
}

type innerIndexer struct {
	kv kvstore.KVStore
}

func (i *innerIndexer) Find(ctx context.Context, sid abi.SectorID) (core.SectorAccessStores, bool, error) {
	var stores core.SectorAccessStores
	// string(b) will copy the underlying bytes, so we use View here
	err := i.kv.Peek(ctx, makeSectorKeySealedFile(sid), func(b []byte) error {
		stores.SealedFile = string(b)
		return nil
	})

	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return stores, false, nil
		}

		return stores, false, fmt.Errorf("locate sealed file: %w", err)
	}

	err = i.kv.Peek(ctx, makeSectorKeyForCacheDir(sid), func(b []byte) error {
		stores.CacheDir = string(b)
		return nil
	})

	if err != nil && !errors.Is(err, kvstore.ErrKeyNotFound) {
		return stores, false, fmt.Errorf("locate cache dir: %w", err)
	}

	if stores.CacheDir == "" {
		stores.CacheDir = stores.SealedFile
	}

	return stores, true, nil
}

func (i *innerIndexer) Update(ctx context.Context, sid abi.SectorID, access core.SectorAccessStores) error {
	if instance := access.SealedFile; instance != "" {
		err := i.kv.Put(ctx, makeSectorKeySealedFile(sid), []byte(instance))
		if err != nil {
			return fmt.Errorf("set sealed file location: %w", err)
		}
	}

	if instance := access.CacheDir; instance != "" {
		err := i.kv.Put(ctx, makeSectorKeyForCacheDir(sid), []byte(instance))
		if err != nil {
			return fmt.Errorf("set cache dir location: %w", err)
		}
	}

	return nil
}
