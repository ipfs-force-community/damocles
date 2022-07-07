package objstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/mroth/weightedrand"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var mgrLog = logging.New("objstore-mgr")

var _ Manager = (*StoreManager)(nil)
var storeReserveSummaryKey = kvstore.Key("StoreReserveSummary")

// for storage allocator
type StoreReserveSummary struct {
	Stats map[string]*StoreReserveStat
}

func emptyStoreReserveSummary() StoreReserveSummary {
	return StoreReserveSummary{
		Stats: map[string]*StoreReserveStat{},
	}
}

type StoreReserveStat struct {
	ReservedSize uint64

	Reserved map[string]StoreReserved
}

func emptyStoreReserveStat() *StoreReserveStat {
	return &StoreReserveStat{
		ReservedSize: 0,
		Reserved:     map[string]StoreReserved{},
	}
}

type StoreReserved struct {
	By   string
	Size uint64
	At   int64
}

type StoreInfo struct {
	Instance InstanceInfo
	Reserved StoreReserveStat
}

type Manager interface {
	GetInstance(ctx context.Context, name string) (Store, error)
	ListInstances(ctx context.Context) ([]StoreInfo, error)
	ReserveSpace(ctx context.Context, by abi.SectorID, size uint64, candidates []string) (*Config, error)
	ReleaseReserved(ctx context.Context, by abi.SectorID) (bool, error)
}

type StoreSelectPolicy struct {
	AllowMiners []abi.ActorID
	DenyMiners  []abi.ActorID
}

func (p StoreSelectPolicy) Allowed(miner abi.ActorID) bool {
	if len(p.DenyMiners) > 0 {
		for _, deny := range p.DenyMiners {
			if deny == miner {
				return false
			}
		}
	}

	if len(p.AllowMiners) > 0 {
		for _, allow := range p.AllowMiners {
			if allow == miner {
				return true
			}
		}

		return false
	}

	return true
}

func NewStoreManager(stores []Store, policy map[string]StoreSelectPolicy, metadb kvstore.KVStore) (*StoreManager, error) {
	idxes := map[string]int{}
	for i, st := range stores {
		idxes[st.Instance(context.Background())] = i
	}

	mgr := &StoreManager{
		storeIdxes: idxes,
		stores:     stores,
		policy:     policy,

		resRand: rand.New(rand.NewSource(time.Now().UnixNano())),
		metadb:  metadb,
	}

	return mgr, nil
}

type StoreManager struct {
	storeIdxes map[string]int
	stores     []Store
	policy     map[string]StoreSelectPolicy

	resRand   *rand.Rand
	reserveMu sync.Mutex
	metadb    kvstore.KVStore
}

func (m *StoreManager) GetInstance(ctx context.Context, name string) (Store, error) {
	idx, ok := m.storeIdxes[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrObjectStoreInstanceNotFound, name)
	}

	return m.stores[idx], nil
}

func (m *StoreManager) ListInstances(ctx context.Context) ([]StoreInfo, error) {
	infos := make([]StoreInfo, 0, len(m.stores))
	err := m.modifyReserved(ctx, func(summary *StoreReserveSummary) (bool, error) {
		for _, store := range m.stores {
			insName := store.Instance(ctx)
			insInfo, err := store.InstanceInfo(ctx)
			if err != nil {
				mgrLog.Errorf("get instance info for %s: %s", insName, err)
				continue
			}

			reserved, ok := summary.Stats[insName]
			if !ok {
				reserved = emptyStoreReserveStat()
			}

			infos = append(infos, StoreInfo{
				Instance: insInfo,
				Reserved: *reserved,
			})
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get reserved info: %w", err)
	}

	return infos, nil
}

type storeCandidate struct {
	Store
	InstanceInfo
}

func (m *StoreManager) ReserveSpace(ctx context.Context, sid abi.SectorID, size uint64, candidates []string) (*Config, error) {
	by := util.FormatSectorID(sid)
	rlog := mgrLog.With("by", by, "size", size)

	var cand map[string]bool
	if len(candidates) > 0 {
		cand = map[string]bool{}
		for _, c := range candidates {
			cand[c] = true
		}
	}

	var selected *Config
	err := m.modifyReserved(ctx, func(summary *StoreReserveSummary) (bool, error) {
		changed := false
		candStores := make([]weightedrand.Choice, 0, len(m.stores))
		for si := range m.stores {
			st := m.stores[si]
			insName := st.Instance(ctx)

			if policy, ok := m.policy[insName]; ok {
				if !policy.Allowed(sid.Miner) {
					continue
				}
			}

			if len(cand) == 0 || cand[insName] {
				info, err := st.InstanceInfo(ctx)
				if err != nil {
					rlog.Errorf("get instance info for %s: %s", insName, err)
					continue
				}

				var resSize uint64
				if resStat, ok := summary.Stats[insName]; ok {
					resSize = resStat.ReservedSize
					// already reserved before
					if reserved, ok := resStat.Reserved[by]; ok && reserved.Size == size {
						rlog.Debugw("already reserved before", "ins", insName)
						selected = &info.Config
						return false, nil
					}
				}

				// readonly, or not enough space
				if info.Config.ReadOnly || info.Free < resSize+size {
					continue
				}

				weight := info.Config.Weight
				if weight == 0 {
					weight = 1
				}

				candStores = append(candStores, weightedrand.Choice{
					Item: storeCandidate{
						Store:        st,
						InstanceInfo: info,
					},
					Weight: weight,
				})
			}
		}

		if len(candStores) == 0 {
			return changed, nil
		}

		selector, err := weightedrand.NewChooser(candStores...)
		if err != nil {
			rlog.Errorf("construct weighted selector: %s", err)
			return false, nil
		}

		chosen := selector.PickSource(m.resRand).(storeCandidate)
		selected = &chosen.Config

		resStat, ok := summary.Stats[chosen.InstanceInfo.Config.Name]
		if !ok {
			resStat = emptyStoreReserveStat()
			summary.Stats[chosen.InstanceInfo.Config.Name] = resStat
			changed = true
		}

		// TODO: check if there already exists a reserved record?
		resStat.Reserved[by] = StoreReserved{
			By:   by,
			Size: size,
			At:   time.Now().Unix(),
		}

		resStat.ReservedSize += size

		changed = true

		return changed, nil
	})

	if err != nil {
		return nil, fmt.Errorf("attempt to select a store: %w", err)
	}

	if selected != nil {
		rlog.Debugw("store selected", "ins", selected.Name)
	} else {
		rlog.Debug("no available store")
	}

	return selected, nil
}

func (m *StoreManager) ReleaseReserved(ctx context.Context, sid abi.SectorID) (bool, error) {
	by := util.FormatSectorID(sid)
	released := false
	err := m.modifyReserved(ctx, func(summary *StoreReserveSummary) (bool, error) {
		changed := false
		for _, stat := range summary.Stats {
			if rev, ok := stat.Reserved[by]; ok {
				if stat.ReservedSize > rev.Size {
					stat.ReservedSize -= rev.Size
				} else {
					stat.ReservedSize = 0
				}
				delete(stat.Reserved, by)
				changed = true
				released = true
			}
		}

		return changed, nil
	})

	if err != nil {
		return false, fmt.Errorf("release reserved: %w", err)
	}

	return released, nil
}

func (m *StoreManager) modifyReserved(ctx context.Context, modifier func(*StoreReserveSummary) (bool, error)) error {
	m.reserveMu.Lock()
	defer m.reserveMu.Unlock()

	var summary StoreReserveSummary
	err := m.metadb.View(ctx, storeReserveSummaryKey, func(data []byte) error {
		return json.Unmarshal(data, &summary)
	})

	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return fmt.Errorf("view store reserve summary: %w", err)
		}

		summary = emptyStoreReserveSummary()
	}

	changed, err := modifier(&summary)
	if err != nil {
		return fmt.Errorf("modify store reserve summary: %w", err)
	}

	if !changed {
		return nil
	}

	b, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal modified store reserve summary: %w", err)
	}

	err = m.metadb.Put(ctx, storeReserveSummaryKey, b)
	if err != nil {
		return fmt.Errorf("save modified store reserve summary: %w", err)
	}

	return nil
}
