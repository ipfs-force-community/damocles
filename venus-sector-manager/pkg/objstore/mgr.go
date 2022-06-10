package objstore

import (
	"context"
	"fmt"
)

type Manager interface {
	GetInstance(ctx context.Context, name string) (Store, error)
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
