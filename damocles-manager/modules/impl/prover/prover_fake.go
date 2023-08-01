package prover

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

func NewFakeVerifier() core.Verifier {
	return &fakeVerifier{}
}

type fakeVerifier struct {
}

func (fakeVerifier) VerifySeal(context.Context, core.SealVerifyInfo) (bool, error) {
	return false, nil
}

func (fakeVerifier) VerifyAggregateSeals(context.Context, core.AggregateSealVerifyProofAndInfos) (bool, error) {
	return false, nil
}

func (fakeVerifier) VerifyWindowPoSt(ctx context.Context, info core.WindowPoStVerifyInfo) (bool, error) {
	return false, nil
}

func (fakeVerifier) VerifyWinningPoSt(ctx context.Context, info core.WinningPoStVerifyInfo) (bool, error) {
	return false, nil
}

func NewFakeProver() core.Prover {
	return &fakeProver{}
}

type fakeProver struct {
}

func (fakeProver) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}

func (fakeProver) GenerateWindowPoSt(ctx context.Context, params core.GenerateWindowPoStParams) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {
	return nil, nil, nil
}

func (fakeProver) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, proofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return nil, nil
}

func (fakeProver) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return nil, nil
}

func (fakeProver) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return nil, nil
}

func (fakeProver) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	return nil, nil
}
