package sectors

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-state-types/abi"
	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
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

var _ core.SectorStateManager = (*StateManager)(nil)

func NewStateManager(online kvstore.KVStore, offline kvstore.KVStore, plugins *managerplugin.LoadedPlugins) (*StateManager, error) {
	return &StateManager{
		online:  online,
		offline: offline,
		plugins: plugins,
		locker: &sectorsLocker{
			sectors: map[abi.SectorID]*sectorLocker{},
		},
	}, nil
}

type StateManager struct {
	online  kvstore.KVStore
	offline kvstore.KVStore

	plugins *managerplugin.LoadedPlugins

	locker *sectorsLocker
}

func (sm *StateManager) pickStore(ws core.SectorWorkerState) (kvstore.KVStore, error) {
	switch ws {
	case core.WorkerOnline:
		return sm.online, nil
	case core.WorkerOffline:
		return sm.offline, nil
	default:
		return nil, fmt.Errorf("kv for worker state %s does not exist", ws)
	}
}

func (sm *StateManager) load(ctx context.Context, key kvstore.Key, state *core.SectorState, ws core.SectorWorkerState) error {
	kv, err := sm.pickStore(ws)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	if err := kv.Peek(ctx, key, func(content []byte) error {
		return json.Unmarshal(content, state)
	}); err != nil {
		return fmt.Errorf("load state from %s: %w", ws, err)
	}

	return nil
}

func (sm *StateManager) save(ctx context.Context, key kvstore.Key, state core.SectorState, ws core.SectorWorkerState) error {
	kv, err := sm.pickStore(ws)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return kv.Put(ctx, key, b)
}

func (sm *StateManager) getIter(ctx context.Context, ws core.SectorWorkerState) (kvstore.Iter, error) {
	kv, err := sm.pickStore(ws)
	if err != nil {
		return nil, fmt.Errorf("get iter: %w", err)
	}

	return kv.Scan(ctx, nil)
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

		if state.MatchWorkerJob(job) {
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

		if !state.MatchWorkerJob(job) {
			continue
		}

		if err := fn(state); err != nil {
			return fmt.Errorf("handle sector state for %s: %w", util.FormatSectorID(state.ID), err)
		}
	}

	return nil
}

func (sm *StateManager) Import(ctx context.Context, ws core.SectorWorkerState, state *core.SectorState, override bool) (bool, error) {
	lock := sm.locker.lock(state.ID)
	defer lock.unlock()

	key := makeSectorKey(state.ID)
	if !override {
		kv, err := sm.pickStore(ws)
		if err != nil {
			return false, fmt.Errorf("import: %w", err)
		}

		err = kv.Peek(ctx, key, func([]byte) error { return nil })
		if err == nil {
			return false, nil
		}

		if err != kvstore.ErrKeyNotFound {
			return false, err
		}
	}

	err := sm.save(ctx, key, *state, ws)
	if err != nil {
		return false, err
	}

	_ = sm.plugins.Foreach(managerplugin.SyncSectorState, func(p *managerplugin.Plugin) error {
		m := managerplugin.DeclareSyncSectorStateManifest(p.Manifest)
		if m.OnImport == nil {
			return nil
		}
		if err := m.OnImport(ws, state, override); err != nil {
			log.Errorf("call plugin OnImport '%s': %w", p.Name, err)
		}
		return nil
	})

	return true, nil
}

func (sm *StateManager) Init(ctx context.Context, sectors []*core.AllocatedSector, ws core.SectorWorkerState) error {
	return sm.InitWith(ctx, sectors, ws)
}

func (sm *StateManager) InitWith(ctx context.Context, sectors []*core.AllocatedSector, ws core.SectorWorkerState, fieldvals ...interface{}) error {
	sids := make([]abi.SectorID, len(sectors))
	for i, s := range sectors {
		sids[i] = s.ID
	}
	locks := sm.locker.lockBatch(sids)
	defer locks.Unlock()

	kv, err := sm.pickStore(ws)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	kvExtend := kvstore.NewExtendKV(kv)
	err = kvExtend.MustNoConflict(func() error {
		return kv.Update(ctx, func(txn kvstore.Txn) error {
			for _, sector := range sectors {
				state := core.SectorState{
					ID:         sector.ID,
					SectorType: sector.ProofType,
				}
				key := makeSectorKey(sector.ID)
				err = kv.Peek(ctx, key, func([]byte) error { return nil })
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

				if err = sm.save(ctx, key, state, ws); err != nil {
					return err
				}
			}

			_ = sm.plugins.Foreach(managerplugin.SyncSectorState, func(p *managerplugin.Plugin) error {
				m := managerplugin.DeclareSyncSectorStateManifest(p.Manifest)
				if m.OnInit == nil {
					return nil
				}
				if err := m.OnInit(sectors, ws); err != nil {
					log.Errorf("call plugin OnInit '%s': %w", p.Name, err)
				}
				return nil
			})
			return nil
		})
	})

	return err
}

func (sm *StateManager) Load(ctx context.Context, sid abi.SectorID, ws core.SectorWorkerState) (*core.SectorState, error) {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state core.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state, ws); err != nil {
		return nil, err
	}

	return &state, nil
}

func (sm *StateManager) Update(ctx context.Context, sid abi.SectorID, ws core.SectorWorkerState, fieldvals ...interface{}) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	var state core.SectorState
	key := makeSectorKey(sid)
	if err := sm.load(ctx, key, &state, ws); err != nil {
		return err
	}

	err := apply(ctx, &state, fieldvals...)
	if err != nil {
		return fmt.Errorf("apply field vals: %w", err)
	}

	if err := sm.save(ctx, key, state, ws); err != nil {
		return err
	}

	_ = sm.plugins.Foreach(managerplugin.SyncSectorState, func(p *managerplugin.Plugin) error {
		m := managerplugin.DeclareSyncSectorStateManifest(p.Manifest)
		if m.OnUpdate == nil {
			return nil
		}
		if err := m.OnUpdate(sid, ws, fieldvals); err != nil {
			log.Errorf("call plugin OnInit '%s': %w", p.Name, err)
		}
		return nil
	})
	return nil
}

func (sm *StateManager) Finalize(ctx context.Context, sid abi.SectorID, onFinalize core.SectorStateChangeHook) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	key := makeSectorKey(sid)
	var state core.SectorState
	if err := sm.load(ctx, key, &state, core.WorkerOnline); err != nil {
		return fmt.Errorf("load from online store: %w", err)
	}

	if onFinalize != nil {
		should, err := onFinalize(&state)
		if err != nil {
			return fmt.Errorf("callback failed before finalize: %w", err)
		}

		if !should {
			return nil
		}
	}

	state.Finalized = true
	if err := sm.save(ctx, key, state, core.WorkerOffline); err != nil {
		return fmt.Errorf("save info into offline store: %w", err)
	}

	if err := sm.online.Del(ctx, key); err != nil {
		return fmt.Errorf("del from online store: %w", err)
	}

	_ = sm.plugins.Foreach(managerplugin.SyncSectorState, func(p *managerplugin.Plugin) error {
		m := managerplugin.DeclareSyncSectorStateManifest(p.Manifest)
		if m.OnFinalize == nil {
			return nil
		}
		if err := m.OnFinalize(sid); err != nil {
			log.Errorf("call plugin OnFinalize '%s': %w", p.Name, err)
		}
		return nil
	})

	return nil
}

func (sm *StateManager) Restore(ctx context.Context, sid abi.SectorID, onRestore core.SectorStateChangeHook) error {
	lock := sm.locker.lock(sid)
	defer lock.unlock()

	key := makeSectorKey(sid)
	var state core.SectorState
	if err := sm.load(ctx, key, &state, core.WorkerOffline); err != nil {
		return fmt.Errorf("load from offline store: %w", err)
	}

	if onRestore != nil {
		should, err := onRestore(&state)
		if err != nil {
			return fmt.Errorf("callback failed before restore: %w", err)
		}

		if !should {
			return nil
		}
	}

	state.Finalized = false
	if err := sm.save(ctx, key, state, core.WorkerOnline); err != nil {
		return fmt.Errorf("save info into online store: %w", err)
	}

	if err := sm.offline.Del(ctx, key); err != nil {
		return fmt.Errorf("del from offline store: %w", err)
	}

	_ = sm.plugins.Foreach(managerplugin.SyncSectorState, func(p *managerplugin.Plugin) error {
		m := managerplugin.DeclareSyncSectorStateManifest(p.Manifest)
		if m.OnRestore == nil {
			return nil
		}
		if err := m.OnRestore(sid); err != nil {
			log.Errorf("call plugin OnRestore '%s': %w", p.Name, err)
		}
		return nil
	})

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
