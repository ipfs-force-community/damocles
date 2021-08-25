package api

import (
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

type Verifier interface {
	VerifySeal(proof5.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error)
}

type Prover interface {
	AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
}
