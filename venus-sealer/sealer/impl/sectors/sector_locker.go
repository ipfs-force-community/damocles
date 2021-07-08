package sectors

import (
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
)

func newSectorsLocker() *sectorsLocker {
	return &sectorsLocker{
		sectors: map[abi.SectorID]*sectorLocker{},
	}
}

type sectorLocker struct {
	mu   sync.Mutex
	last time.Time
}

func (sl *sectorLocker) unlock() {
	sl.mu.Unlock()
}

type sectorsLocker struct {
	sync.Mutex
	sectors map[abi.SectorID]*sectorLocker
}

func (sl *sectorsLocker) lock(sid abi.SectorID) *sectorLocker {
	sl.Lock()
	lock, ok := sl.sectors[sid]
	if !ok {
		lock = &sectorLocker{}
		sl.sectors[sid] = lock
	}

	lock.last = time.Now()
	sl.Unlock()

	lock.mu.Lock()
	return lock
}
