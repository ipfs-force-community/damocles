package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var _ core.WorkerManager = (*Manager)(nil)

func makeWorkerKey(name string) kvstore.Key {
	return kvstore.Key(name)
}

func NewManager(kv kvstore.KVStore) (*Manager, error) {
	return &Manager{
		kv: kv,
	}, nil
}

type Manager struct {
	kv kvstore.KVStore
}

func (m *Manager) Load(ctx context.Context, name string) (core.WorkerPingInfo, error) {
	key := makeWorkerKey(name)

	var winfo core.WorkerPingInfo
	if err := m.kv.View(ctx, key, func(content []byte) error {
		return json.Unmarshal(content, &winfo)
	}); err != nil {
		return winfo, fmt.Errorf("load worker info: %w", err)
	}

	return winfo, nil
}

func (m *Manager) Update(ctx context.Context, winfo core.WorkerPingInfo) error {
	b, err := json.Marshal(winfo)
	if err != nil {
		return fmt.Errorf("marshal worker info: %w", err)
	}

	key := makeWorkerKey(winfo.Info.Name)

	return m.kv.Put(ctx, key, b)
}

func (m *Manager) All(ctx context.Context, filter func(*core.WorkerPingInfo) bool) ([]core.WorkerPingInfo, error) {
	iter, err := m.kv.Scan(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get iter: %w", err)
	}

	defer iter.Close()

	infos := make([]core.WorkerPingInfo, 0, 32)
	for iter.Next() {
		var winfo core.WorkerPingInfo
		if err := iter.View(ctx, func(data []byte) error {
			return json.Unmarshal(data, &winfo)
		}); err != nil {
			return nil, fmt.Errorf("scan state item of key %s: %w", string(iter.Key()), err)
		}

		if filter == nil || filter(&winfo) {
			infos = append(infos, winfo)
		}

	}

	return infos, nil
}

func (m *Manager) Remove(ctx context.Context, name string) error {
	key := makeWorkerKey(name)
	if err := m.kv.Del(ctx, key); err != nil {
		return fmt.Errorf("try to remove the worker meta %s: %w", name, err)
	}
	return nil
}
