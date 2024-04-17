package sectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

var _ core.RebuildSectorManager = (*RebuildManager)(nil)

var rebuildInfoKey = kvstore.Key("rebuild-infos")

type RebuildInfos struct {
	Actors []RebuildInfosForActor
}

type RebuildInfosForActor struct {
	Actor abi.ActorID
	Infos map[abi.SectorNumber]core.SectorRebuildInfo
}

func NewRebuildManager(
	scfg *modules.SafeConfig,
	minerAPI core.MinerAPI,
	infoKVStore kvstore.KVStore,
) (*RebuildManager, error) {
	return &RebuildManager{
		msel: newMinerSelector(scfg, minerAPI),
		kv:   infoKVStore,
	}, nil
}

type RebuildManager struct {
	msel *minerSelector

	kvMu sync.Mutex
	kv   kvstore.KVStore
}

func (rm *RebuildManager) Set(ctx context.Context, sid abi.SectorID, info core.SectorRebuildInfo) error {
	err := rm.loadAndUpdate(ctx, func(infos *RebuildInfos) bool {
		added := false
		for i := range infos.Actors {
			if infos.Actors[i].Actor == sid.Miner {
				if infos.Actors[i].Infos == nil {
					infos.Actors[i].Infos = map[abi.SectorNumber]core.SectorRebuildInfo{}
				}

				infos.Actors[i].Infos[sid.Number] = info
				added = true
				break
			}
		}

		if !added {
			infos.Actors = append(infos.Actors, RebuildInfosForActor{
				Actor: sid.Miner,
				Infos: map[abi.SectorNumber]core.SectorRebuildInfo{
					sid.Number: info,
				},
			})
		}

		return true
	})
	if err != nil {
		return fmt.Errorf("add rebuild info: %w", err)
	}

	return nil
}

func (rm *RebuildManager) Allocate(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorRebuildInfo, error) {
	cands := rm.msel.candidates(
		ctx,
		spec.AllowedMiners,
		spec.AllowedProofTypes,
		func(mcfg modules.MinerConfig) bool { return true },
		"rebuild",
	)
	if len(cands) == 0 {
		return nil, nil
	}

	allowed := map[abi.ActorID]struct{}{}
	for _, cand := range cands {
		allowed[cand.info.ID] = struct{}{}
	}

	var allocated *core.SectorRebuildInfo
	err := rm.loadAndUpdate(ctx, func(infos *RebuildInfos) bool {
		if len(infos.Actors) == 0 {
			return false
		}

		for ai := range infos.Actors {
			if _, allow := allowed[infos.Actors[ai].Actor]; allow && len(infos.Actors[ai].Infos) > 0 {
				for num := range infos.Actors[ai].Infos {
					info := infos.Actors[ai].Infos[num]
					allocated = &info
					delete(infos.Actors[ai].Infos, num)
					return true
				}
			}
		}

		return false
	})
	if err != nil {
		return nil, fmt.Errorf("allocate rebuid info: %w", err)
	}

	return allocated, nil
}

func (rm *RebuildManager) loadAndUpdate(ctx context.Context, modify func(infos *RebuildInfos) bool) error {
	rm.kvMu.Lock()
	defer rm.kvMu.Unlock()

	var infos RebuildInfos
	err := rm.kv.Peek(ctx, rebuildInfoKey, func(v kvstore.Val) error {
		verr := json.Unmarshal(v, &infos)
		if verr != nil {
			return fmt.Errorf("unmashal rebuild infos: %w", verr)
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return fmt.Errorf("load rebuild infos: %w", err)
		}
	}

	updated := modify(&infos)
	if !updated {
		return nil
	}

	val, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("marshal rebuild infos: %w", err)
	}

	err = rm.kv.Put(ctx, rebuildInfoKey, val)
	if err != nil {
		return fmt.Errorf("put data of rebuild infos: %w", err)
	}

	return nil
}
