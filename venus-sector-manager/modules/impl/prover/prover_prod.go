//go:build prod
// +build prod

package prover

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

type verifier struct {
}

func (verifier) VerifySeal(ctx context.Context, svi core.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(svi)
}

func (verifier) VerifyAggregateSeals(ctx context.Context, aggregate core.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info core.WindowPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWindowPoSt(info)
}

func (verifier) VerifyWinningPoSt(ctx context.Context, info core.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWinningPoSt(info)
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}

func (prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {
	randomness[31] &= 0x3f
	proof, faulty, err := ffi.GenerateWindowPoSt(minerID, sectors, randomness)

	var faultyIDs []abi.SectorID
	for _, f := range faulty {
		faultyIDs = append(faultyIDs, abi.SectorID{
			Miner:  minerID,
			Number: f,
		})
	}

	return proof, faultyIDs, err
}

func (prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	randomness[31] &= 0x3f

	return ffi.GenerateWinningPoSt(minerID, sectors, randomness)
}

func (prover) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	randomness[31] &= 0x3f
	return ffi.GeneratePoStFallbackSectorChallenges(proofType, minerID, randomness, sectorIds)
}

func (prover) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return ffi.GenerateSingleVanillaProof(replica, challenges)
}

func (prover) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	randomness[31] &= 0x3f
	return ffi.GenerateWinningPoStWithVanilla(proofType, minerID, randomness, proofs)
}
