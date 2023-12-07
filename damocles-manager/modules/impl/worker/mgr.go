package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/metrics"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("worker-manager")
var _ core.WorkerManager = (*Manager)(nil)

func makeWorkerKey(name string) kvstore.Key {
	return kvstore.Key(name)
}

func NewManager(ctx context.Context, kv kvstore.KVStore) (*Manager, error) {
	// launch a thread to record metrics
	mgr := &Manager{
		kv: kv,
	}

	mgr.doMetrics(ctx)
	return mgr, nil
}

type Manager struct {
	kv kvstore.KVStore
}

func (m *Manager) Load(ctx context.Context, name string) (core.WorkerPingInfo, error) {
	key := makeWorkerKey(name)

	var winfo core.WorkerPingInfo
	if err := m.kv.Peek(ctx, key, func(content []byte) error {
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

func (m *Manager) doMetrics(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 60)
	go func() {
		for {
			select {
			case <-ticker.C:
				infos, err := m.All(ctx, nil)
				if err != nil {
					log.Warnf("get all workers: %s", err)
					continue
				}

				workerLatencyCount := map[string]int64{"latency<=60s": 0, "latency<=120s": 0, "latency<=300s": 0, "latency>300s": 0}
				threadStateCount := map[string]int64{}
				now := time.Now().Unix()
				for _, info := range infos {
					latency := now - info.LastPing
					if latency <= 60 {
						workerLatencyCount["latency<=60s"]++
					} else if latency <= 120 {
						workerLatencyCount["latency<=120s"]++
					} else if latency <= 300 {
						workerLatencyCount["latency<=300s"]++
					} else {
						workerLatencyCount["latency>300s"]++
					}

					stateCount := info.Info.Summary.Map()
					for k, v := range stateCount {
						if _, ok := threadStateCount[k]; !ok {
							threadStateCount[k] = 0
						}
						threadStateCount[k] += int64(v)
					}
				}

				for k, v := range threadStateCount {
					metrics.ThreadCount.Set(ctx, k, v)
				}

				for k, v := range workerLatencyCount {
					metrics.WorkerLatencyCount.Set(ctx, k, v)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
