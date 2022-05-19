package sectors

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var stateFields []reflect.StructField

func init() {
	rst := reflect.TypeOf(core.SectorState{})
	fnum := rst.NumField()
	fields := make([]reflect.StructField, 0, fnum)
	for fi := 0; fi < fnum; fi++ {
		field := rst.Field(fi)
		fields = append(fields, field)
	}

	stateFields = fields
}

func WorkerJobMatcher(jobType core.SectorWorkerJob, state *core.SectorState) bool {
	switch jobType {
	case core.SectorWorkerJobAll:
		return true

	case core.SectorWorkerJobSealing:
		return !bool(state.Upgraded)

	case core.SectorWorkerJobSnapUp:
		return bool(state.Upgraded)

	default:
		return false
	}
}

var _ core.SectorStateManager = (*StateManager)(nil)

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

func (sm *StateManager) load(ctx context.Context, key kvstore.Key, state *core.SectorState, online bool) error {
	var (
		kv    kvstore.KVStore
		kvSrc string
	)

	if online {
		kv = sm.online
		kvSrc = "online"
	} else {
		kv = sm.offline
		kvSrc = "offline"
	}

	if err := kv.View(ctx, key, func(content []byte) error {
		return json.Unmarshal(content, state)
	}); err != nil {
		return fmt.Errorf("load state from %s: %w", kvSrc, err)
	}

	return nil
}

func (sm *StateManager) save(ctx context.Context, key kvstore.Key, state core.SectorState, online bool) error {
	var kv kvstore.KVStore
	if online {
		kv = sm.online
	} else {
		kv = sm.offline
	}

	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return kv.Put(ctx, key, b)
}

func (sm *StateManager) getIter(ctx context.Context, ws core.SectorWorkerState) (kvstore.Iter, error) {
	switch ws {
	case core.WorkerOnline:
		return sm.online.Scan(ctx, nil)
	case core.WorkerOffline:
		return sm.offline.Scan(ctx, nil)

	default:
		return nil, fmt.Errorf("unexpected state store type %s", ws)
	}
}

func (sm *StateManager) All(ctx context.Context, ws core.SectorWorkerState, job core.SectorWorkerJob) ([]*core.SectorState, error) {
	iter, err := sm.getIter(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("get iter for All: %w", err)
	}

	defer iter.Close()

	states := make([]*core.SectorState, 0, 32)
	for iter.Next() {
		var state core.SectorState
		if err := iter.View(ctx, func(data []byte) error {
			return json.Unmarshal(data, &state)
		}); err != nil {
			return nil, fmt.Errorf("scan state item of key %s: %w", string(iter.Key()), err)
		}

		if WorkerJobMatcher(job, &state) {
			states = append(states, &state)
		}
	}

	return states, nil
}

func (sm *StateManager) ForEach(ctx context.Context, ws core.SectorWorkerState, job core.SectorWorkerJob, fn func(core.SectorState) error) error {
	iter, err := sm.getIter(ctx, ws)
	if err != nil {
		return fmt.Errorf("get iter for ForEach: %w", err)
	}

	defer iter.Close()

	for iter.Next() {
		var state core.SectorState
		if err := iter.View(ctx, func(data []byte) error {
			return json.Unmarshal(data, &state)
		}); err != nil {
			return fmt.Errorf("scan state item of key %s: %w", string(iter.Key()), err)
		}

		if !WorkerJobMatcher(job, &state) {
			continue
		}

		if err := fn(state); err != nil {
			return fmt.Errorf("handle sector state for %s: %w", util.FormatSectorID(state.ID), err)
		}
	}

	return nil
}

func (sm *StateManager) Init(ctx context.Context, sid abi.SectorID, st abi.RegisteredSealProof) error {
	return sm.InitWith(ctx, sid, st)
}

func (sm *StateManager) InitWith(ctx context.Context, sid abi.SectorID, proofType abi.RegisteredSealProof, fieldvals ...interface{}) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	state := core.SectorState{
		ID:         sid,
		SectorType: proofType,
	}

	key := makeSectorKey(sid)
	err := sm.online.View(ctx, key, func([]byte) error { return nil })
	if err == nil {
		return fmt.Errorf("sector %s already initialized", string(key))
	}

	if err != kvstore.ErrKeyNotFound {
		return err
	}

	if len(fieldvals) > 0 {
		err = apply(ctx, &state, fieldvals...)
		if err != nil {
			return fmt.Errorf("apply field vals: %w", err)
		}
	}

	return sm.save(ctx, key, state, true)
}

func (sm *StateManager) Load(ctx context.Context, sid abi.SectorID, online bool) (*core.SectorState, error) {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state core.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state, online); err != nil {
		return nil, err
	}

	return &state, nil
}

func (sm *StateManager) Update(ctx context.Context, sid abi.SectorID, online bool, fieldvals ...interface{}) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state core.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state, online); err != nil {
		return err
	}

	err := apply(ctx, &state, fieldvals...)
	if err != nil {
		return fmt.Errorf("apply field vals: %w", err)
	}

	return sm.save(ctx, key, state, online)
}

func (sm *StateManager) Finalize(ctx context.Context, sid abi.SectorID, onFinalize core.SectorStateChangeHook) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	key := makeSectorKey(sid)
	var state core.SectorState
	if err := sm.load(ctx, key, &state, true); err != nil {
		return fmt.Errorf("load from online store: %w", err)
	}

	if onFinalize != nil {
		should, err := onFinalize(&state)
		if err != nil {
			return fmt.Errorf("callback falied before finalize: %w", err)
		}

		if !should {
			return nil
		}
	}

	state.Finalized = true
	if err := sm.save(ctx, key, state, false); err != nil {
		return fmt.Errorf("save info into offline store: %w", err)
	}

	if err := sm.online.Del(ctx, key); err != nil {
		return fmt.Errorf("del from online store: %w", err)
	}

	return nil
}

func (sm *StateManager) Restore(ctx context.Context, sid abi.SectorID, onRestore core.SectorStateChangeHook) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	key := makeSectorKey(sid)
	var state core.SectorState
	if err := sm.load(ctx, key, &state, false); err != nil {
		return fmt.Errorf("load from offline store: %w", err)
	}

	if onRestore != nil {
		should, err := onRestore(&state)
		if err != nil {
			return fmt.Errorf("callback falied before restore: %w", err)
		}

		if !should {
			return nil
		}
	}

	state.Finalized = false
	if err := sm.save(ctx, key, state, true); err != nil {
		return fmt.Errorf("save info into online store: %w", err)
	}

	if err := sm.offline.Del(ctx, key); err != nil {
		return fmt.Errorf("del from offline store: %w", err)
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

func apply(ctx context.Context, state *core.SectorState, fieldvals ...interface{}) error {
	statev := reflect.ValueOf(state).Elem()
	for fi := range fieldvals {
		fieldval := fieldvals[fi]
		if err := processStateField(statev, fieldval); err != nil {
			return err
		}
	}

	return nil
}
