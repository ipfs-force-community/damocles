package ext

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("ext-prover")

var _ core.Prover = (*Prover)(nil)

func New(
	ctx context.Context,
	sectorTracker core.SectorTracker,
	windowCfgs []extproc.ExtProcessorConfig,
	winningCfgs []extproc.ExtProcessorConfig,
) (*Prover, error) {
	var windowProc, winningPorc *extproc.Processor
	var err error
	if len(windowCfgs) > 0 {
		windowProc, err = extproc.New(ctx, stage.NameWindowPoSt, windowCfgs)
		if err != nil {
			return nil, fmt.Errorf("construct WindowPoSt Processor: %w", err)
		}

		log.Info("WindowPoSt ext prover constructed")
	}

	if len(winningCfgs) > 0 {
		winningPorc, err = extproc.New(ctx, stage.NameWinningPost, winningCfgs)
		if err != nil {
			return nil, fmt.Errorf("construct WinningPoSt Processor: %w", err)
		}

		log.Info("WinningPoSt ext prover constructed")
	}

	if windowProc == nil && winningPorc == nil {
		return nil, fmt.Errorf("no ext prover constructed")
	}

	return &Prover{
		sectorTracker: sectorTracker,
		localProver:   prover.NewProdProver(sectorTracker),
		windowProc:    windowProc,
		winningPorc:   winningPorc,
	}, nil
}

type Prover struct {
	sectorTracker core.SectorTracker
	localProver   core.Prover
	windowProc    *extproc.Processor
	winningPorc   *extproc.Processor
}

func (p *Prover) Run() {
	if p.windowProc != nil {
		p.windowProc.Run()
	}

	if p.winningPorc != nil {
		p.winningPorc.Run()
	}
}

func (p *Prover) Close() {
	if p.windowProc != nil {
		p.windowProc.Close()
	}

	if p.winningPorc != nil {
		p.winningPorc.Close()
	}
}

func (p *Prover) AggregateSealProofs(
	ctx context.Context,
	aggregateInfo core.AggregateSealVerifyProofAndInfos,
	proofs [][]byte,
) ([]byte, error) {
	return p.localProver.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *Prover) GenerateWindowPoSt(
	ctx context.Context,
	params core.GenerateWindowPoStParams,
) ([]builtin.PoStProof, []abi.SectorID, error) {
	if p.windowProc == nil {
		return p.localProver.GenerateWindowPoSt(ctx, params)
	}
	minerID, proofType, sectors, randomness := params.MinerID, params.ProofType, params.Sectors, params.Randomness
	randomness[31] &= 0x3f

	if len(sectors) == 0 {
		return nil, nil, nil
	}
	privSectors, err := p.sectorTracker.PubToPrivate(ctx, minerID, proofType, sectors)
	if err != nil {
		return nil, nil, fmt.Errorf("turn public sector infos into private: %w", err)
	}

	data := stage.WindowPoSt{
		MinerID:   minerID,
		ProofType: stage.ProofType2String(proofType),
	}
	copy(data.Seed[:], randomness[:])

	for i := range privSectors {
		inner := privSectors[i]

		if pt := inner.PoStProofType; pt != proofType {
			return nil, nil, fmt.Errorf(
				"proof type not match for sector %d of miner %d: want %s, got %s",
				inner.SectorNumber,
				minerID,
				stage.ProofType2String(proofType),
				stage.ProofType2String(pt),
			)
		}

		commR, err := util.CID2ReplicaCommitment(inner.SealedCID)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"invalid selaed cid %s for sector %d of miner %d: %w",
				inner.SealedCID,
				inner.SectorNumber,
				minerID,
				err,
			)
		}

		data.Replicas = append(data.Replicas, stage.PoStReplicaInfo{
			SectorID:   inner.SectorNumber,
			CommR:      commR,
			CacheDir:   inner.CacheDirPath,
			SealedFile: inner.SealedSectorPath,
		})
	}

	var res stage.WindowPoStOutput

	err = p.windowProc.Process(ctx, data, &res)
	if err != nil {
		return nil, nil, fmt.Errorf("WindowPoStProcessor.Process: %w", err) //revive:disable-line:error-strings
	}

	if faultCount := len(res.Faults); faultCount != 0 {
		faults := make([]abi.SectorID, faultCount)
		for fi := range res.Faults {
			faults[fi] = abi.SectorID{
				Miner:  minerID,
				Number: res.Faults[fi],
			}
		}

		return nil, faults, fmt.Errorf("got %d fault sectors", faultCount)
	}

	proofs := make([]builtin.PoStProof, len(res.Proofs))
	for pi := range res.Proofs {
		proofs[pi] = builtin.PoStProof{
			PoStProof:  proofType,
			ProofBytes: res.Proofs[pi],
		}
	}
	return proofs, nil, nil
}

func (p *Prover) GenerateWinningPoSt(
	ctx context.Context,
	minerID abi.ActorID,
	proofType abi.RegisteredPoStProof,
	sectors []builtin.ExtendedSectorInfo,
	randomness abi.PoStRandomness,
) ([]builtin.PoStProof, error) {
	randomness[31] &= 0x3f
	if p.winningPorc == nil {
		return p.localProver.GenerateWinningPoSt(ctx, minerID, proofType, sectors, randomness)
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	data := stage.WinningPost{
		MinerID:   minerID,
		ProofType: stage.ProofType2String(proofType),
	}
	copy(data.Seed[:], randomness[:])

	privSectors, err := p.sectorTracker.PubToPrivate(ctx, minerID, proofType, sectors)
	if err != nil {
		return nil, fmt.Errorf("turn public sector infos into private: %w", err)
	}

	for i := range privSectors {
		inner := privSectors[i]

		if pt := inner.PoStProofType; pt != proofType {
			return nil, fmt.Errorf(
				"proof type not match for sector %d of miner %d: want %s, got %s",
				inner.SectorNumber,
				minerID,
				stage.ProofType2String(proofType),
				stage.ProofType2String(pt),
			)
		}

		commR, err := util.CID2ReplicaCommitment(inner.SealedCID)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid sealed cid %s for sector %d of miner %d: %w",
				inner.SealedCID,
				inner.SectorNumber,
				minerID,
				err,
			)
		}

		data.Replicas = append(data.Replicas, stage.PoStReplicaInfo{
			SectorID:   inner.SectorNumber,
			CommR:      commR,
			CacheDir:   inner.CacheDirPath,
			SealedFile: inner.SealedSectorPath,
		})
	}

	var res stage.WinningPoStOutput

	err = p.winningPorc.Process(ctx, data, &res)
	if err != nil {
		return nil, fmt.Errorf("WinningPoStProcessor.Process: %w", err) //revive:disable-line:error-strings
	}

	proofs := make([]builtin.PoStProof, len(res.Proofs))
	for pi := range res.Proofs {
		proofs[pi] = builtin.PoStProof{
			PoStProof:  proofType,
			ProofBytes: res.Proofs[pi],
		}
	}

	return proofs, nil
}

func (p *Prover) GeneratePoStFallbackSectorChallenges(
	ctx context.Context,
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	sectorIds []abi.SectorNumber,
) (*core.FallbackChallenges, error) {
	randomness[31] &= 0x3f
	return p.localProver.GeneratePoStFallbackSectorChallenges(ctx, proofType, minerID, randomness, sectorIds)
}

func (p *Prover) GenerateSingleVanillaProof(
	ctx context.Context,
	replica core.FFIPrivateSectorInfo,
	challenges []uint64,
) ([]byte, error) {
	return p.localProver.GenerateSingleVanillaProof(ctx, replica, challenges)
}

func (p *Prover) GenerateWinningPoStWithVanilla(
	ctx context.Context,
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
) ([]core.PoStProof, error) {
	randomness[31] &= 0x3f
	return p.localProver.GenerateWinningPoStWithVanilla(ctx, proofType, minerID, randomness, proofs)
}
