// +build !prod

package prover

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"
)

type verifier struct {
}

func (verifier) VerifySeal(context.Context, proof7.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (verifier) VerifyAggregateSeals(context.Context, proof7.AggregateSealVerifyProofAndInfos) (bool, error) {
	return true, nil
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info proof7.WindowPoStVerifyInfo) (bool, error) {
	return true, nil
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo proof7.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}

func (prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []proof7.PoStProof, skipped []abi.SectorID, err error) {
	return nil, nil, nil
}

func (prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof7.PoStProof, error) {
	return nil, nil
}
