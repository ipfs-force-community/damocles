// +build prod

package prover

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

type verifier struct {
}

func (verifier) VerifySeal(svi proof5.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(svi)
}

func (verifier) VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

type prover struct {
}

func (prover) AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}
