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

func (fakeVerifier) VerifyWindowPoSt(_ context.Context, _ core.WindowPoStVerifyInfo) (bool, error) {
	return false, nil
}

func (fakeVerifier) VerifyWinningPoSt(_ context.Context, _ core.WinningPoStVerifyInfo) (bool, error) {
	return false, nil
}

func NewFakeProver() core.Prover {
	return &fakeProver{}
}

type fakeProver struct {
}

func (fakeProver) AggregateSealProofs(context.Context, core.AggregateSealVerifyProofAndInfos, [][]byte) ([]byte, error) {
	return make([]byte, 32), nil
}

func (fakeProver) GenerateWindowPoSt(context.Context, core.GenerateWindowPoStParams) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {
	return nil, nil, nil
}

func (fakeProver) GenerateWinningPoSt(context.Context, abi.ActorID, abi.RegisteredPoStProof, []builtin.ExtendedSectorInfo, abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return nil, nil
}

func (fakeProver) GeneratePoStFallbackSectorChallenges(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return nil, nil
}

func (fakeProver) GenerateSingleVanillaProof(context.Context, core.FFIPrivateSectorInfo, []uint64) ([]byte, error) {
	return nil, nil
}

func (fakeProver) GenerateWinningPoStWithVanilla(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, [][]byte) ([]core.PoStProof, error) {
	return nil, nil
}
