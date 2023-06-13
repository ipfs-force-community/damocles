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

type sectorLocks []*sectorLocker

func (ls sectorLocks) Lock() {
	for _, l := range ls {
		l.mu.Lock()
	}
}

func (ls sectorLocks) Unlock() {
	for _, l := range ls {
		l.mu.Unlock()
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

func (sl *sectorsLocker) lockBatch(sids []abi.SectorID) sectorLocks {
	sl.Lock()

	now := time.Now()
	locks := sectorLocks(make([]*sectorLocker, len(sids)))
	for i, sid := range sids {
		lock, ok := sl.sectors[sid]
		if !ok {
			lock = &sectorLocker{}
			sl.sectors[sid] = lock
		}

		lock.last = now
		locks[i] = lock
	}
	sl.Unlock()

	locks.Lock()
	return locks
}
