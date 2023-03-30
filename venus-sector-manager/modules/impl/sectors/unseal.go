package sectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	"github.com/google/uuid"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var unsealInfoKey = kvstore.Key("unseal-infos")

type UnsealInfos struct {
	Allocatable map[abi.ActorID]map[abi.SectorNumber]*core.SectorUnsealInfo
	Allocated   map[uuid.UUID]*core.SectorUnsealInfo
}

// UnsealManager manage unseal task
type UnsealManager struct {
	msel *minerSelector
	kvMu sync.Mutex
	kv   kvstore.KVStore
}

var _ core.UnsealSectorManager = (*UnsealManager)(nil)

func NewUnsealManager(ctx context.Context, scfg *modules.SafeConfig, minfoAPI core.MinerInfoAPI, kv kvstore.KVStore, mEvent market.IMarketEvent) (*UnsealManager, error) {
	ret := &UnsealManager{
		kv:   kv,
		msel: newMinerSelector(scfg, minfoAPI),
	}

	// register to market event
	mEvent.OnUnseal(func(ctx context.Context, eventId vtypes.UUID, req *market.UnsealRequest) {
		actor, err := address.IDFromAddress(req.Miner)
		if err != nil {
			log.Errorf("get miner id from address: %s", err)
			return
		}
		err = ret.Set(ctx, &core.SectorUnsealInfo{
			SectorID: abi.SectorID{
				Miner:  abi.ActorID(actor),
				Number: req.Sid,
			},
			PieceCid: req.PieceCid,
			Offset:   req.Offset,
			Size:     req.Size,
		})
		if err != nil {
			log.Errorf("set unseal info: %s", err)
		}
	})
	return ret, nil

}

// Set set unseal task
func (u *UnsealManager) Set(ctx context.Context, req *core.SectorUnsealInfo) error {
	// check piece store
	// todo: if exist in piece store , respond directly

	// set into db
	err := u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {
		info, ok := infos.Allocatable[req.SectorID.Miner]
		if !ok {
			info = map[abi.SectorNumber]*core.SectorUnsealInfo{}
		}
		info[req.SectorID.Number] = req
		infos.Allocatable[req.SectorID.Miner] = info
		return true
	})

	if err != nil {
		return fmt.Errorf("add unseal info: %w", err)
	}
	return nil
}

// allocate a unseal task
func (u *UnsealManager) Allocate(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorUnsealInfo, error) {
	cands := u.msel.candidates(ctx, spec.AllowedMiners, spec.AllowedProofTypes, func(mcfg modules.MinerConfig) bool { return true }, "unseal")
	if len(cands) == 0 {
		return nil, nil
	}

	// read db
	var allocated *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {

		if len(infos.Allocatable) == 0 {
			return false
		}

		for _, candidate := range cands {
			info, ok := infos.Allocatable[candidate.info.ID]
			if !ok {
				continue
			}
			if len(info) == 0 {
				continue
			}
			for sectorNum, v := range info {
				allocated = v
				delete(infos.Allocatable[candidate.info.ID], sectorNum)
				infos.Allocated[allocated.Id] = allocated
				return true
			}
		}

		return false
	})

	if err != nil {
		return nil, fmt.Errorf("allocate unseal info: %w", err)
	}

	return allocated, nil
}

// archive a unseal task
func (u *UnsealManager) Archive(ctx context.Context, evenId uuid.UUID) error {
	// respond to market
	var info *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {
		ok := false
		info, ok = infos.Allocated[evenId]
		if !ok {
			return false
		}
		delete(infos.Allocated, evenId)
		return true
	})

	if err != nil {
		return fmt.Errorf("archive unseal info(actor=%s sector=%s event_id=%s ): %w", info.SectorID.Miner, info.SectorID.Number, info.Id, err)
	}

	// todo: build transfer task and do it

	return nil
}

func (u *UnsealManager) loadAndUpdate(ctx context.Context, modify func(infos *UnsealInfos) bool) error {
	u.kvMu.Lock()
	defer u.kvMu.Unlock()

	var infos UnsealInfos
	err := u.kv.View(ctx, unsealInfoKey, func(v kvstore.Val) error {
		verr := json.Unmarshal(v, &infos)
		if verr != nil {
			return fmt.Errorf("unmashal unseal infos: %w", verr)
		}

		return nil
	})

	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return fmt.Errorf("load unseal infos: %w", err)
		}
	}

	if infos.Allocatable == nil {
		infos.Allocatable = make(map[abi.ActorID]map[abi.SectorNumber]*core.SectorUnsealInfo)
	}
	if infos.Allocated == nil {
		infos.Allocated = make(map[uuid.UUID]*core.SectorUnsealInfo)
	}

	updated := modify(&infos)
	if !updated {
		return nil
	}

	val, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("marshal unseal infos: %w", err)
	}

	err = u.kv.Put(ctx, unsealInfoKey, val)
	if err != nil {
		return fmt.Errorf("put data of unseal infos: %w", err)
	}

	return nil
}
