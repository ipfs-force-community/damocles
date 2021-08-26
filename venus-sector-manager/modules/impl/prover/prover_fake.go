// +build !prod

package prover

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

type verifier struct {
}

func (verifier) VerifySeal(context.Context, proof5.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (verifier) VerifyAggregateSeals(context.Context, proof5.AggregateSealVerifyProofAndInfos) (bool, error) {
	return true, nil
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info proof5.WindowPoStVerifyInfo) (bool, error) {
	return true, nil
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}

func (prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []proof5.PoStProof, skipped []abi.SectorID, err error) {
	return nil, nil, nil
}
