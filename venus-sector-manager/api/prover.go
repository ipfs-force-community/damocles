package api

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
)

type SectorRef = storage.SectorRef

type SectorOnChainInfo = miner.SectorOnChainInfo

type PrivateSectorInfo struct {
	Accesses         SectorAccessStores
	CacheDirPath     string
	SealedSectorPath string

	CacheDirURI     string
	SealedSectorURI string
}

func (p PrivateSectorInfo) ToFFI(sector builtin.SectorInfo, proofType abi.RegisteredPoStProof) FFIPrivateSectorInfo {
	return FFIPrivateSectorInfo{
		SectorInfo:       sector,
		CacheDirPath:     p.CacheDirPath,
		PoStProofType:    proofType,
		SealedSectorPath: p.SealedSectorPath,
	}
}

type (
	FFIPrivateSectorInfo    = ffi.PrivateSectorInfo
	SortedPrivateSectorInfo = ffi.SortedPrivateSectorInfo
)

var (
	NewSortedPrivateSectorInfo = ffi.NewSortedPrivateSectorInfo
)

type Verifier interface {
	VerifySeal(context.Context, SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(ctx context.Context, aggregate AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info WindowPoStVerifyInfo) (bool, error)
}

type Prover interface {
	AggregateSealProofs(ctx context.Context, aggregateInfo AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
	GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []builtin.PoStProof, skipped []abi.SectorID, err error)
	GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error)
}
