package api

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"
)

type (
	PrivateSectorInfo       = ffi.PrivateSectorInfo
	SortedPrivateSectorInfo = ffi.SortedPrivateSectorInfo
	WindowPoStVerifyInfo    = proof7.WindowPoStVerifyInfo
)

var (
	NewSortedPrivateSectorInfo = ffi.NewSortedPrivateSectorInfo
)

type Verifier interface {
	VerifySeal(context.Context, proof7.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(ctx context.Context, aggregate proof7.AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof7.WindowPoStVerifyInfo) (bool, error)
}

type Prover interface {
	AggregateSealProofs(ctx context.Context, aggregateInfo proof7.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
	GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []proof7.PoStProof, skipped []abi.SectorID, err error)
	GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof7.PoStProof, error)
}
