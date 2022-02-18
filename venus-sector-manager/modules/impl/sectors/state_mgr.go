package sectors

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var stateFields []reflect.StructField

func init() {
	rst := reflect.TypeOf(api.SectorState{})
	fnum := rst.NumField()
	fields := make([]reflect.StructField, 0, fnum)
	for fi := 0; fi < fnum; fi++ {
		field := rst.Field(fi)
		fields = append(fields, field)
	}

	stateFields = fields
}

var _ api.SectorStateManager = (*StateManager)(nil)

func NewStateManager(online kvstore.KVStore, offline kvstore.KVStore) (*StateManager, error) {
	return &StateManager{
		online:  online,
		offline: offline,
		locker: &sectorsLocker{
			sectors: map[abi.SectorID]*sectorLocker{},
		},
	}, nil
}

type StateManager struct {
	online  kvstore.KVStore
	offline kvstore.KVStore

	locker *sectorsLocker
}

func (sm *StateManager) load(ctx context.Context, key kvstore.Key, state *api.SectorState) error {
	if err := sm.online.View(ctx, key, func(content []byte) error {
		return json.Unmarshal(content, state)
	}); err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	return nil
}

func (sm *StateManager) All(ctx context.Context, ws api.SectorWorkerState) ([]*api.SectorState, error) {
	var (
		iter kvstore.Iter
		err  error
	)

	switch ws {
	case api.WorkerOnline:
		iter, err = sm.online.Scan(ctx, nil)
	case api.WorkerOffline:
		iter, err = sm.offline.Scan(ctx, nil)
	}
	if err != nil {
		return nil, err
	}

	defer iter.Close()

	states := make([]*api.SectorState, 0, 32)  // TODO 只返回了32个?
	for iter.Next() {
		var state api.SectorState
		if err := iter.View(ctx, func(data []byte) error {
			return json.Unmarshal(data, &state)
		}); err != nil {
			return nil, fmt.Errorf("scan state item of key %s: %w", string(iter.Key()), err)
		}

		states = append(states, &state)
	}

	return states, nil
}

func (sm *StateManager) Init(ctx context.Context, sid abi.SectorID, st abi.RegisteredSealProof) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	state := api.SectorState{
		ID:         sid,
		SectorType: st,
	}

	key := makeSectorKey(sid)
	err := sm.online.View(ctx, key, func([]byte) error { return nil })
	if err == nil {
		return fmt.Errorf("sector %s already initialized", string(key))
	}

	if err != kvstore.ErrKeyNotFound {
		return err
	}

	return save(ctx, sm.online, key, state)
}

func (sm *StateManager) Load(ctx context.Context, sid abi.SectorID) (*api.SectorState, error) {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state api.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func (sm *StateManager) Update(ctx context.Context, sid abi.SectorID, fieldvals ...interface{}) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state api.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state); err != nil {
		return err
	}

	statev := reflect.ValueOf(&state).Elem()
	for fi := range fieldvals {
		fieldval := fieldvals[fi]
		if err := processStateField(statev, fieldval); err != nil {
			return err
		}
	}

	return save(ctx, sm.online, key, state)
}

func (sm *StateManager) Finalize(ctx context.Context, sid abi.SectorID, onFinalize func(*api.SectorState) error) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	key := makeSectorKey(sid)
	var state api.SectorState
	if err := sm.load(ctx, key, &state); err != nil {
		return fmt.Errorf("load from online store: %w", err)
	}

	if onFinalize != nil {
		err := onFinalize(&state)
		if err != nil {
			return fmt.Errorf("callback falied before finalize: %w", err)
		}
	}

	state.Finalized = true
	if err := save(ctx, sm.offline, key, state); err != nil {
		return fmt.Errorf("save info into offline store: %w", err)
	}

	if err := sm.online.Del(ctx, key); err != nil {
		return fmt.Errorf("del from online store: %w", err)
	}

	return nil
}

func processStateField(rv reflect.Value, fieldval interface{}) error {
	rfv := reflect.ValueOf(fieldval)
	// most likely, reflect.ValueOf(nil)
	if !rfv.IsValid() {
		return fmt.Errorf("invalid field value: %s", rfv)
	}

	rft := rfv.Type()

	for i, sf := range stateFields {
		if sf.Type == rft {
			rv.Field(i).Set(rfv)
			return nil
		}
	}

	return fmt.Errorf("field not found for type %s", rft)
}

func makeSectorKey(sid abi.SectorID) kvstore.Key {
	return []byte(fmt.Sprintf("m-%d-n-%d", sid.Miner, sid.Number))
}

func save(ctx context.Context, store kvstore.KVStore, key kvstore.Key, state api.SectorState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return store.Put(ctx, key, b)
}
