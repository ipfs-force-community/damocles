// +build prod

package prover

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"
)

type verifier struct {
}

func (verifier) VerifySeal(ctx context.Context, svi proof7.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(svi)
}

func (verifier) VerifyAggregateSeals(ctx context.Context, aggregate proof7.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info proof7.WindowPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWindowPoSt(info)
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo proof7.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}

func (prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []proof7.PoStProof, skipped []abi.SectorID, err error) {
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

func (p prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof7.PoStProof, error) {
	randomness[31] &= 0x3f

	return ffi.GenerateWinningPoSt(minerID, sectors, randomness)
}
