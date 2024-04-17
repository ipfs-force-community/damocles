package prover

import (
	"context"
	"fmt"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("prover")

func NewProdVerifier() core.Verifier {
	return &prodVerifier{}
}

type prodVerifier struct{}

func (prodVerifier) VerifySeal(_ context.Context, svi core.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(svi)
}

func (prodVerifier) VerifyAggregateSeals(
	_ context.Context,
	aggregate core.AggregateSealVerifyProofAndInfos,
) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

func (prodVerifier) VerifyWindowPoSt(_ context.Context, info core.WindowPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWindowPoSt(info)
}

func (prodVerifier) VerifyWinningPoSt(_ context.Context, info core.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return ffi.VerifyWinningPoSt(info)
}

func NewProdProver(sectorTracker core.SectorTracker) core.Prover {
	return &prodProver{
		sectorTracker: sectorTracker,
	}
}

type prodProver struct {
	sectorTracker core.SectorTracker
}

func (prodProver) AggregateSealProofs(
	_ context.Context,
	aggregateInfo core.AggregateSealVerifyProofAndInfos,
	proofs [][]byte,
) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}

func (p prodProver) GenerateWindowPoSt(
	ctx context.Context,
	params core.GenerateWindowPoStParams,
) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {
	minerID, proofType, sectors, randomness := params.MinerID, params.ProofType, params.Sectors, params.Randomness
	randomness[31] &= 0x3f

	privSectors, err := p.sectorTracker.PubToPrivate(ctx, minerID, proofType, sectors)
	if err != nil {
		return nil, nil, fmt.Errorf("turn public sector infos into private: %w", err)
	}

	proof, faulty, err := ffi.GenerateWindowPoSt(minerID, core.NewSortedPrivateSectorInfo(privSectors...), randomness)

	faultyIDs := make([]abi.SectorID, len(faulty))
	for i, f := range faulty {
		faultyIDs[i] = abi.SectorID{
			Miner:  minerID,
			Number: f,
		}
	}

	return proof, faultyIDs, err
}

func (p prodProver) GenerateWinningPoSt(
	ctx context.Context,
	minerID abi.ActorID,
	ppt abi.RegisteredPoStProof,
	sectors []builtin.ExtendedSectorInfo,
	randomness abi.PoStRandomness,
) ([]builtin.PoStProof, error) {
	randomness[31] &= 0x3f

	privSectors, err := p.sectorTracker.PubToPrivate(ctx, minerID, ppt, sectors)
	if err != nil {
		return nil, fmt.Errorf("turn public sector infos into private: %w", err)
	}

	return ffi.GenerateWinningPoSt(minerID, core.NewSortedPrivateSectorInfo(privSectors...), randomness)
}

func (prodProver) GeneratePoStFallbackSectorChallenges(
	_ context.Context,
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	sectorIds []abi.SectorNumber,
) (*core.FallbackChallenges, error) {
	randomness[31] &= 0x3f
	return ffi.GeneratePoStFallbackSectorChallenges(proofType, minerID, randomness, sectorIds)
}

func (prodProver) GenerateSingleVanillaProof(
	ctx context.Context,
	replica core.FFIPrivateSectorInfo,
	challenges []uint64,
) ([]byte, error) {
	start := time.Now()

	resCh := make(chan core.Result[[]byte], 1)
	go func() {
		resCh <- core.Wrap(ffi.GenerateSingleVanillaProof(replica, challenges))
	}()

	select {
	case r := <-resCh:
		return r.Unwrap()
	case <-ctx.Done():
		log.Errorw(
			"failed to generate valilla PoSt proof before context cancellation",
			"err",
			ctx.Err(),
			"duration",
			time.Since(start),
			"cache",
			replica.CacheDirPath,
			"sealed",
			replica.SealedSectorPath,
		)

		// this will leave the GenerateSingleVanillaProof goroutine hanging, but that's still less bad than failing PoSt
		return nil, fmt.Errorf("failed to generate vanilla proof before context cancellation: %w", ctx.Err())
	}
}

func (prodProver) GenerateWinningPoStWithVanilla(
	_ context.Context,
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
) ([]core.PoStProof, error) {
	randomness[31] &= 0x3f
	return ffi.GenerateWinningPoStWithVanilla(proofType, minerID, randomness, proofs)
}
