package mock

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

var _ core.SectorManager = (*sectorMgr)(nil)

func NewSectorManager(miner abi.ActorID, proofType abi.RegisteredSealProof) core.SectorManager {
	return &sectorMgr{
		miner:     miner,
		proofType: proofType,
		sectorNum: 0,
	}
}

type sectorMgr struct {
	miner     abi.ActorID
	proofType abi.RegisteredSealProof
	sectorNum uint64
}

func (s *sectorMgr) Allocate(ctx context.Context, spec core.AllocateSectorSpec, count uint32) ([]*core.AllocatedSector, error) {
	allowedMiners := spec.AllowedMiners
	allowedProofTypes := spec.AllowedProofTypes
	if count == 0 {
		return nil, fmt.Errorf("at least one sector is allocated each call")
	}

	minerFits := len(allowedMiners) == 0
	for _, want := range allowedMiners {
		if want == s.miner {
			minerFits = true
			break
		}
	}

	if !minerFits {
		return nil, nil
	}

	typeFits := len(allowedProofTypes) == 0
	for _, typ := range allowedProofTypes {
		if typ == s.proofType {
			typeFits = true
			break
		}
	}

	if !typeFits {
		return nil, nil
	}

	next := atomic.AddUint64(&s.sectorNum, uint64(count))

	var (
		i  uint32
		id = next - uint64(count) + 1
	)
	allocatedSectors := make([]*core.AllocatedSector, count)
	for i < count {
		allocatedSectors[i] = &core.AllocatedSector{
			ID: abi.SectorID{
				Miner:  s.miner,
				Number: abi.SectorNumber(id),
			},
			ProofType: s.proofType,
		}
		i++
		id++
	}

	if count == 1 {
		log.Infow("sector allocated", "sector_id", next)
	} else {
		log.Infow("sector allocated", "sector_ids", fmt.Sprintf("%d-%d", next-uint64(count)+1, next))
	}

	return allocatedSectors, nil
}
