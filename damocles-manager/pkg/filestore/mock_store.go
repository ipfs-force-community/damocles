package filestore

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
)

var _ Store = (*MockStore)(nil)

func NewMockStore(cfg Config, size uint64) (Store, error) {
	if size == 0 {
		return nil, fmt.Errorf("zero-sized store is not valid")
	}

	return &MockStore{
		cfg:     cfg,
		size:    size,
		used:    0,
		objects: map[string]*bytes.Buffer{},
	}, nil
}

type MockStore struct {
	cfg     Config
	size    uint64
	used    uint64
	objects map[string]*bytes.Buffer
	mu      sync.RWMutex
}

func (ms *MockStore) Type() string {
	return "mock"
}

func (ms *MockStore) Version() string {
	return "mock"
}

func (ms *MockStore) Instance(context.Context) string {
	return ms.cfg.Name
}

func (ms *MockStore) InstanceInfo(context.Context) (InstanceInfo, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return InstanceInfo{
		Config:      ms.cfg,
		Type:        "mock",
		Total:       ms.size,
		Free:        ms.size - ms.used,
		Used:        ms.used,
		UsedPercent: float64(ms.used) * 100 / float64(ms.size),
	}, nil
}

func (ms *MockStore) InstanceConfig(context.Context) Config {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.cfg
}

func (ms *MockStore) SubPath(_ context.Context, pathType PathType, sectorID *SectorID, custom *string) (subPath string, err error) {
	if pathType == PathTypeCustom {
		if custom == nil {
			return "", fmt.Errorf("sectorID cannot be nil")
		}
		return *custom, nil
	}

	if sectorID == nil {
		return "", fmt.Errorf("sectorID cannot be nil")
	}

	sid := abi.SectorID{
		Miner:  abi.ActorID(sectorID.Miner),
		Number: abi.SectorNumber(sectorID.Number),
	}

	switch pathType {
	case PathTypeSealed:
		subPath = util.SectorPath(util.SectorPathTypeSealed, sid)
	case PathTypeUpdate:
		subPath = util.SectorPath(util.SectorPathTypeUpdate, sid)
	case PathTypeCache:
		subPath = util.SectorPath(util.SectorPathTypeCache, sid)
	case PathTypeUpdateCache:
		subPath = util.SectorPath(util.SectorPathTypeUpdateCache, sid)
	default:
		return "", fmt.Errorf("unsupport path type: %s", pathType)
	}

	return subPath, nil
}
