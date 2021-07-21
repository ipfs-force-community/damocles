package sectors

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/kvstore"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
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

type StateManager struct {
	store kvstore.KVStore

	locker *sectorsLocker
}

func (sm *StateManager) save(ctx context.Context, key kvstore.Key, state api.SectorState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return sm.store.Put(ctx, key, b)
}

func (sm *StateManager) load(ctx context.Context, key kvstore.Key, state *api.SectorState) error {
	if err := sm.store.View(ctx, key, func(content []byte) error {
		return json.Unmarshal(content, state)
	}); err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	return nil
}

func (sm *StateManager) All(ctx context.Context) ([]*api.SectorState, error) {
	iter, err := sm.store.Scan(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer iter.Close()

	states := make([]*api.SectorState, 0, 32)
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

func (sm *StateManager) Init(ctx context.Context, sid abi.SectorID) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	state := api.SectorState{
		ID: sid,
	}

	key := makeSectorKey(sid)
	err := sm.store.View(ctx, key, func([]byte) error { return nil })
	if err == nil {
		return fmt.Errorf("sector %s already initialized", string(key))
	}

	if err != kvstore.ErrKeyNotFound {
		return err
	}

	return sm.save(ctx, key, state)
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

	return sm.save(ctx, key, state)
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
