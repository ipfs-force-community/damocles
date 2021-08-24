package filestore

import (
	"context"
	"fmt"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/objstore"
)

var _ objstore.Manager = (*Manager)(nil)

func NewManager(cfgs []Config) (*Manager, error) {
	stores := map[string]*Store{}

	for _, cfg := range cfgs {
		store, err := Open(cfg)
		if err != nil {
			return nil, err
		}

		stores[store.cfg.Name] = store
	}

	return &Manager{
		stores: stores,
	}, nil
}

type Manager struct {
	stores map[string]*Store
}

func (m *Manager) GetInstance(ctx context.Context, name string) (objstore.Store, error) {
	store, ok := m.stores[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", objstore.ErrObjectStoreInstanceNotFound, name)
	}

	return store, nil
}
