package objstore

import (
	"context"
	"fmt"
)

// for storage allocator
type StoreSummary struct {
	Stats map[string]StoreStat
}

type StoreStat struct {
	ReservedSize uint64
	Weight       int64

	Reserved map[string]StoreReserved
}

type StoreReserved struct {
	Sector string
	Size   uint64
	At     int64
}

type Manager interface {
	GetInstance(ctx context.Context, name string) (Store, error)
	// ReserveSpace(ctx context.Context, by string, size uint64, instanceChoices []string) (*InstanceMeta, error)
	// ReleaseReserved(ctx context.Context, by string) error
}

var _ Manager = (*StoreManager)(nil)

func NewStoreManager(stores []Store) (*StoreManager, error) {
	mgr := &StoreManager{
		stores: map[string]Store{},
	}

	for _, store := range stores {
		mgr.stores[store.Instance(context.Background())] = store
	}

	return mgr, nil
}

type StoreManager struct {
	stores map[string]Store
}

func (m *StoreManager) GetInstance(ctx context.Context, name string) (Store, error) {
	store, ok := m.stores[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrObjectStoreInstanceNotFound, name)
	}

	return store, nil
}
