package sectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	market8 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

func sectorGoodForSnapup(sinfo *miner.SectorOnChainInfo, currentHeight abi.ChainEpoch) bool {
	return sinfo.SectorKeyCID == nil && len(sinfo.DeprecatedDealIDs) == 0 &&
		sinfo.Expiration-currentHeight >= market8.DealMinDuration
}

func kvKeyForMinerActorID(mid abi.ActorID) kvstore.Key {
	return kvstore.Key(strconv.FormatUint(uint64(mid), 10))
}

func NewSnapUpAllocator(
	chainAPI chain.API,
	minerAPI core.MinerAPI,
	allocStore kvstore.KVStore,
	indexer core.SectorIndexer,
	scfg *modules.SafeConfig,
) (*SnapUpAllocator, error) {
	allocator := &SnapUpAllocator{
		chain: chainAPI,

		msel: newMinerSelector(scfg, minerAPI),

		kv: allocStore,

		indexer: indexer,
	}

	return allocator, nil
}

type SnapUpAllocator struct {
	chain chain.API

	msel *minerSelector

	kvMu sync.Mutex
	kv   kvstore.KVStore

	indexer core.SectorIndexer
}

func (s *SnapUpAllocator) PreFetch(
	ctx context.Context,
	mid abi.ActorID,
	dlindex *uint64,
) (count uint64, diff uint64, err error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid miner actor id %d: %w", mid, err)
	}

	ts, err := s.chain.ChainHead(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("get chain head: %w", err)
	}

	tsk := ts.Key()
	tsh := ts.Height()

	deadline, err := s.chain.StateMinerProvingDeadline(ctx, maddr, tsk)
	if err != nil {
		return 0, 0, fmt.Errorf("get proving deadline: %w", err)
	}

	dlidx := uint64(0)
	if dlindex != nil {
		dlidx = *dlindex % deadline.WPoStPeriodDeadlines
	} else {
		dlidx = deadline.Index
	}

	partitions, err := s.chain.StateMinerPartitions(ctx, maddr, dlidx, tsk)
	if err != nil {
		return 0, 0, fmt.Errorf("get partitions: %w", err)
	}

	actives := bitfield.New()

	for pi := range partitions {
		psectors, err := s.chain.StateMinerSectors(ctx, maddr, &partitions[pi].ActiveSectors, tsk)
		if err != nil {
			return 0, 0, fmt.Errorf("get #%d partition info: %w", pi, err)
		}

		for _, sinfo := range psectors {
			if sectorGoodForSnapup(sinfo, tsh) {
				actives.Set(uint64(sinfo.SectorNumber))
			}
		}
	}

	count, err = actives.Count()
	if err != nil {
		return 0, 0, fmt.Errorf("get active count: %w", err)
	}

	if count == 0 {
		return 0, 0, nil
	}

	s.kvMu.Lock()
	defer s.kvMu.Unlock()

	diff, err = s.addSectors(ctx, mid, dlidx, deadline.WPoStPeriodDeadlines, actives, count)
	if err != nil {
		return 0, 0, fmt.Errorf("add active sectors: %w", err)
	}

	return count, diff, nil
}

func (s *SnapUpAllocator) Candidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error) {
	key := kvKeyForMinerActorID(mid)
	exists, err := s.loadExists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load exists: %w", err)
	}

	return exists, nil
}

func (s *SnapUpAllocator) Allocate(ctx context.Context, spec core.AllocateSectorSpec) (*core.SnapUpCandidate, error) {
	mcandidates := s.msel.candidates(
		ctx,
		spec.AllowedMiners,
		spec.AllowedProofTypes,
		func(mcfg modules.MinerConfig) bool {
			return mcfg.SnapUp.Enabled
		},
		"snapup",
	)

	if len(mcandidates) == 0 {
		return nil, nil
	}

	mcandidate := mcandidates[rand.Intn(len(mcandidates))]
	allocatedSector, err := s.allocateForMiner(ctx, mcandidate)
	if err != nil {
		return nil, fmt.Errorf("allocate sector for %d: %w", mcandidate.info.ID, err)
	}

	return allocatedSector, nil
}

type deadlineCandidate struct {
	dlidx int
	count uint64
}

func (s *SnapUpAllocator) allocateForMiner(
	ctx context.Context,
	mcandidate *minerCandidate,
) (*core.SnapUpCandidate, error) {
	mid := mcandidate.info.ID
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return nil, fmt.Errorf("invalid miner actor id: %d: %w", mid, err)
	}

	key := kvKeyForMinerActorID(mid)

	s.kvMu.Lock()
	defer s.kvMu.Unlock()

	exists, err := s.loadExists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load exist sectors: %w", err)
	}

	if exists == nil {
		return nil, nil
	}

	rootLog := log.With("mid", mid)

	candidates := []deadlineCandidate{}

	for dlidx := range exists {
		exist := exists[dlidx]
		if exist == nil {
			continue
		}

		dlog := rootLog.With("deadline", dlidx)

		count, err := exist.Count()
		if err != nil {
			dlog.Warnf("get count of #%d deadline: %s", dlidx, err)
			continue
		}

		if count == 0 {
			continue
		}

		candidates = append(candidates, deadlineCandidate{
			dlidx: dlidx,
			count: count,
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	selectedDeadline := candidates[rand.Intn(len(candidates))]
	selectedBitfield := exists[selectedDeadline.dlidx]
	all, err := selectedBitfield.All(selectedDeadline.count)
	if err != nil {
		return nil, fmt.Errorf("get all numbers from #%d deadline: %w", selectedDeadline.dlidx, err)
	}

	// this is unlikely to happen
	if len(all) == 0 {
		return nil, nil
	}

	selectedNum := all[rand.Intn(len(all))]

	ts, err := s.chain.ChainHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("get chain head: %w", err)
	}

	tsk := ts.Key()
	tsh := ts.Height()

	partitions, err := s.chain.StateMinerPartitions(ctx, maddr, uint64(selectedDeadline.dlidx), tsk)
	if err != nil {
		return nil, fmt.Errorf("get partitions: %w", err)
	}

	isActive := false
	for _, pat := range partitions {
		if has, _ := pat.ActiveSectors.IsSet(selectedNum); has {
			isActive = true
			break
		}
	}

	rootLog = rootLog.With("tsk", tsk.String(), "tsh", tsh, "sector", selectedNum)
	if !isActive {
		rootLog.Warn("not active")
		return nil, nil
	}

	sinfo, err := s.chain.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(selectedNum), tsk)
	if err != nil {
		return nil, fmt.Errorf("get sector info for %d: %w", selectedNum, err)
	}

	exists[selectedDeadline.dlidx].Unset(selectedNum)
	err = s.updateExists(ctx, key, exists)
	if err != nil {
		return nil, fmt.Errorf("save updated exist bitfields after unset %d: %w", selectedNum, err)
	}

	// check after unset, so that the bad sector won't stay
	if !sectorGoodForSnapup(sinfo, tsh) {
		return nil, nil
	}

	commR, err := util.CID2ReplicaCommitment(sinfo.SealedCID)
	if err != nil {
		return nil, fmt.Errorf("invalid sealed cid %s: %w", sinfo.SealedCID, err)
	}

	sid := abi.SectorID{
		Miner:  mid,
		Number: abi.SectorNumber(selectedNum),
	}

	instances, found, err := s.indexer.Normal().Find(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find persist store instance for %s: %w", util.FormatSectorID(sid), err)
	}

	// TODO: if we should allow splitted sectors
	if instances.SealedFile != instances.CacheDir {
		return nil, nil
	}

	if !found {
		return nil, fmt.Errorf("persist store instance not found for %s", util.FormatSectorID(sid))
	}

	return &core.SnapUpCandidate{
		DeadlineIndex: uint64(selectedDeadline.dlidx),
		Sector: core.AllocatedSector{
			ID:        sid,
			ProofType: mcandidate.info.SealProofType,
		},
		Public: core.SectorPublicInfo{
			CommR:      commR,
			SealedCID:  sinfo.SealedCID,
			Activation: sinfo.Activation,
			Expiration: sinfo.Expiration,
		},
		Private: core.SectorPrivateInfo{
			AccessInstance: instances.SealedFile,
		},
	}, nil
}

func (s *SnapUpAllocator) Release(ctx context.Context, candidate *core.SnapUpCandidate) error {
	key := kvKeyForMinerActorID(candidate.Sector.ID.Miner)

	s.kvMu.Lock()
	defer s.kvMu.Unlock()

	exists, err := s.loadExists(ctx, key)
	if err != nil {
		return fmt.Errorf("load exist bitfields: %w", err)
	}

	if exists == nil {
		return fmt.Errorf("no exist bitfields found")
	}

	if existLen := len(exists); uint64(existLen) <= candidate.DeadlineIndex {
		return fmt.Errorf("deadline not matched: %d/%d", candidate.DeadlineIndex, existLen)
	}

	exist := exists[candidate.DeadlineIndex]
	if exist == nil {
		return fmt.Errorf("got nil bitfield")
	}

	exist.Set(uint64(candidate.Sector.ID.Number))
	exists[candidate.DeadlineIndex] = exist
	err = s.updateExists(ctx, key, exists)
	if err != nil {
		return fmt.Errorf("update exists: %w", err)
	}

	return nil
}

func (s *SnapUpAllocator) loadExists(ctx context.Context, key kvstore.Key) ([]*bitfield.BitField, error) {
	var exists []*bitfield.BitField
	err := s.kv.Peek(ctx, key, func(v kvstore.Val) error {
		err := json.Unmarshal(v, &exists)
		if err != nil {
			return fmt.Errorf("bitfield from bytes: %w", err)
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, fmt.Errorf("load exist sectors: %w", err)
		}

		return nil, nil
	}

	return exists, nil
}

func (s *SnapUpAllocator) updateExists(ctx context.Context, key kvstore.Key, exists []*bitfield.BitField) error {
	data, err := json.Marshal(exists)
	if err != nil {
		return fmt.Errorf("marshal exist bitfields: %w", err)
	}

	err = s.kv.Put(ctx, key, data)
	if err != nil {
		return fmt.Errorf("save exist bitfieds: %w", err)
	}

	return nil
}

func (s *SnapUpAllocator) addSectors(
	ctx context.Context,
	mid abi.ActorID,
	dlidx uint64,
	deadlines uint64,
	sectors bitfield.BitField,
	_ uint64,
) (uint64, error) {
	key := kvKeyForMinerActorID(mid)

	exists, err := s.loadExists(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("load exists: %w", err)
	}

	// not found
	if exists == nil {
		exists = make([]*bitfield.BitField, deadlines)
	}

	if int(dlidx) >= len(exists) {
		return 0, fmt.Errorf("deadline index overflow: %d/%d", dlidx, len(exists))
	}

	exist := exists[dlidx]
	existCount := uint64(0)
	if exist != nil {
		existCount, err = exist.Count()
		if err != nil {
			return 0, fmt.Errorf("get exist count: %w", err)
		}
	}

	if existCount == 0 {
		exist = &sectors
	} else {
		merged, err := bitfield.MergeBitFields(*exist, sectors)
		if err != nil {
			return 0, fmt.Errorf("merge sectors: %w", err)
		}

		exist = &merged
	}

	mergedCount, err := exist.Count()
	if err != nil {
		return 0, fmt.Errorf("get merged count: %w", err)
	}

	if mergedCount == existCount {
		return 0, nil
	}

	exists[dlidx] = exist
	err = s.updateExists(ctx, key, exists)
	if err != nil {
		return 0, fmt.Errorf("save merged bitfied: %w", err)
	}

	return mergedCount - existCount, nil
}
