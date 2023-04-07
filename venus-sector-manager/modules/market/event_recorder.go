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

type EventRecorder struct {
	kv    kvstore.KVStore
	kvMux sync.Mutex
}

func (e *EventRecorder) load(ctx context.Context) (*records, error) {
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

func (e *EventRecorder) save(ctx context.Context, r records) error {
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

func (e *EventRecorder) Add(ctx context.Context, id types.UUID, url string) error {
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

func (e *EventRecorder) Remove(ctx context.Context, id types.UUID) error {
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

func (e *EventRecorder) Get(ctx context.Context, id types.UUID) (string, error) {
	r, err := e.load(ctx)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", nil
	}
	return (*r)[id.String()], nil
}
