// +build !prod

package prover

import (
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

type verifier struct {
}

func (verifier) VerifySeal(svi proof5.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (verifier) VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error) {
	return true, nil
}

type prover struct {
}

func (prover) AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}
