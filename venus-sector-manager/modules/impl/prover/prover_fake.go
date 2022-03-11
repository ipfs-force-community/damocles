// +build !prod

package prover

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

type verifier struct {
}

func (verifier) VerifySeal(context.Context, api.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (verifier) VerifyAggregateSeals(context.Context, api.AggregateSealVerifyProofAndInfos) (bool, error) {
	return true, nil
}

func (verifier) VerifyWindowPoSt(ctx context.Context, info api.WindowPoStVerifyInfo) (bool, error) {
	return true, nil
}

type prover struct {
}

func (prover) AggregateSealProofs(ctx context.Context, aggregateInfo api.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}

func (prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {
	return nil, nil, nil
}

func (prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return nil, nil
}
