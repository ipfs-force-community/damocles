package api

import (
	"context"
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

type (
	PrivateSectorInfo       = ffi.PrivateSectorInfo
	SortedPrivateSectorInfo = ffi.SortedPrivateSectorInfo
	WindowPoStVerifyInfo    = proof5.WindowPoStVerifyInfo
)

var (
	NewSortedPrivateSectorInfo = ffi.NewSortedPrivateSectorInfo
)

type Verifier interface {
	VerifySeal(context.Context, proof5.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(ctx context.Context, aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof5.WindowPoStVerifyInfo) (bool, error)
}

type Prover interface {
	AggregateSealProofs(ctx context.Context, aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
	GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []proof5.PoStProof, skipped []abi.SectorID, err error)
	GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof5.PoStProof, error) // TODO nv15 changed
}
