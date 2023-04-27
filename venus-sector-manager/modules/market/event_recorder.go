package market

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var kvStoreKey = kvstore.Key("event_record")

// records trace the eventId and it's gateway client
type records map[string]string

type EventRecorder interface {
	Add(ctx context.Context, id types.UUID, url string) error
	Remove(ctx context.Context, id types.UUID) error
	Get(ctx context.Context, id types.UUID) (string, error)
}

type MemoryEventRecorder struct {
	mux sync.Mutex
	m   map[string]string
}

func NewMemoryEventRecorder() *MemoryEventRecorder {
	return &MemoryEventRecorder{
		m: make(map[string]string),
	}
}

func (e *MemoryEventRecorder) Add(ctx context.Context, id types.UUID, url string) error {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.m[id.String()] = url
	return nil
}

func (e *MemoryEventRecorder) Remove(ctx context.Context, id types.UUID) error {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.m, id.String())
	return nil
}

func (e *MemoryEventRecorder) Get(ctx context.Context, id types.UUID) (string, error) {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.m[id.String()], nil
}

type StoreEventRecorder struct {
	kv    kvstore.KVStore
	kvMux sync.Mutex
}

func (e *StoreEventRecorder) load(ctx context.Context) (*records, error) {
	e.kvMux.Lock()
	defer e.kvMux.Unlock()

	var ret *records
	val, err := e.kv.Get(ctx, kvStoreKey)
	if err != nil {
		return ret, err
	}
	err = json.Unmarshal(val, ret)
	if err != nil {
		return ret, err
	}

	return ret, nil
}

func (e *StoreEventRecorder) save(ctx context.Context, r records) error {
	e.kvMux.Lock()
	defer e.kvMux.Unlock()

	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	err = e.kv.Put(ctx, kvStoreKey, data)
	if err != nil {
		return err
	}
	return nil
}

func (e *StoreEventRecorder) Add(ctx context.Context, id types.UUID, url string) error {
	r, err := e.load(ctx)
	if err != nil {
		return err
	}
	if r == nil {
		r = &records{}
	}
	(*r)[id.String()] = url
	return e.save(ctx, *r)
}

func (e *StoreEventRecorder) Remove(ctx context.Context, id types.UUID) error {
	r, err := e.load(ctx)
	if err != nil {
		return err
	}
	if r == nil {
		return nil
	}
	delete(*r, id.String())
	return e.save(ctx, *r)
}

func (e *StoreEventRecorder) Get(ctx context.Context, id types.UUID) (string, error) {
	r, err := e.load(ctx)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", nil
	}
	return (*r)[id.String()], nil
}
