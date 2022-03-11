// +build prod

package prover

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

type verifier struct {
}

func (verifier) VerifySeal(ctx context.Context, svi api.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(svi)
}

func (verifier) VerifyAggregateSeals(ctx context.Context, aggregate api.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info api.WindowPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWindowPoSt(info)
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo api.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
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

func (p prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	randomness[31] &= 0x3f

	return ffi.GenerateWinningPoSt(minerID, sectors, randomness)
}
