package mock

import (
	"context"
	"sync/atomic"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

var _ api.SectorManager = (*sectorMgr)(nil)

func NewSectorManager(miner abi.ActorID, proofType abi.RegisteredSealProof) api.SectorManager {
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

func (s *sectorMgr) Allocate(ctx context.Context, allowedMiners []abi.ActorID, allowedProofTypes []abi.RegisteredSealProof) (*api.AllocatedSector, error) {
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

	next := atomic.AddUint64(&s.sectorNum, 1)
	log.Infow("sector allocated", "next", next)
	return &api.AllocatedSector{
		ID: abi.SectorID{
			Miner:  s.miner,
			Number: abi.SectorNumber(next),
		},
		ProofType: s.proofType,
	}, nil
}
