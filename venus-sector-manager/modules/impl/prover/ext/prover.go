package ext

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/extproc"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("ext-prover")

var _ core.Prover = (*Prover)(nil)

func New(ctx context.Context, windowCfgs []extproc.ExtProcessorConfig, winningCfgs []extproc.ExtProcessorConfig) (*Prover, error) {
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
		windowProc:  windowProc,
		winningPorc: winningPorc,
	}, nil
}

type Prover struct {
	windowProc  *extproc.Processor
	winningPorc *extproc.Processor
}

func (p *Prover) Run() {
	p.windowProc.Run()
	p.winningPorc.Run()
}

func (p *Prover) Close() {
	p.windowProc.Close()
	p.winningPorc.Close()
}

func (*Prover) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return prover.Prover.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *Prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, []abi.SectorID, error) {
	randomness[31] &= 0x3f

	if p.windowProc == nil {
		return prover.Prover.GenerateWindowPoSt(ctx, minerID, sectors, randomness)
	}

	sectorInners := sectors.Values()
	if len(sectorInners) == 0 {
		return nil, nil, nil
	}

	proofType := sectorInners[0].PoStProofType
	data := stage.WindowPoSt{
		MinerID:   minerID,
		ProofType: stage.ProofType2String(proofType),
	}
	copy(data.Seed[:], randomness[:])

	for i := range sectorInners {
		inner := sectorInners[i]

		if pt := inner.PoStProofType; pt != proofType {
			return nil, nil, fmt.Errorf("proof type not match for sector %d of miner %d: want %s, got %s", inner.SectorNumber, minerID, stage.ProofType2String(proofType), stage.ProofType2String(pt))
		}

		commR, err := util.CID2ReplicaCommitment(inner.SealedCID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid selaed cid %s for sector %d of miner %d: %w", inner.SealedCID, inner.SectorNumber, minerID, err)
		}

		data.Replicas = append(data.Replicas, stage.PoStReplicaInfo{
			SectorID:   inner.SectorNumber,
			CommR:      commR,
			CacheDir:   inner.CacheDirPath,
			SealedFile: inner.SealedSectorPath,
		})
	}

	var res stage.WindowPoStOutput

	err := p.windowProc.Process(ctx, data, &res)
	if err != nil {
		return nil, nil, fmt.Errorf("WindowPoStProcessor.Process: %w", err)
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

func (p *Prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	randomness[31] &= 0x3f
	if p.winningPorc == nil {
		return prover.Prover.GenerateWinningPoSt(ctx, minerID, sectors, randomness)
	}

	sectorInners := sectors.Values()
	if len(sectorInners) == 0 {
		return nil, nil
	}

	proofType := sectorInners[0].PoStProofType
	data := stage.WinningPost{
		MinerID:   minerID,
		ProofType: stage.ProofType2String(proofType),
	}
	copy(data.Seed[:], randomness[:])

	for i := range sectorInners {
		inner := sectorInners[i]

		if pt := inner.PoStProofType; pt != proofType {
			return nil, fmt.Errorf("proof type not match for sector %d of miner %d: want %s, got %s", inner.SectorNumber, minerID, stage.ProofType2String(proofType), stage.ProofType2String(pt))
		}

		commR, err := util.CID2ReplicaCommitment(inner.SealedCID)
		if err != nil {
			return nil, fmt.Errorf("invalid selaed cid %s for sector %d of miner %d: %w", inner.SealedCID, inner.SectorNumber, minerID, err)
		}

		data.Replicas = append(data.Replicas, stage.PoStReplicaInfo{
			SectorID:   inner.SectorNumber,
			CommR:      commR,
			CacheDir:   inner.CacheDirPath,
			SealedFile: inner.SealedSectorPath,
		})
	}

	var res stage.WinningPoStOutput

	err := p.winningPorc.Process(ctx, data, &res)
	if err != nil {
		return nil, fmt.Errorf("WinningPoStProcessor.Process: %w", err)
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

func (*Prover) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return prover.Prover.GeneratePoStFallbackSectorChallenges(ctx, proofType, minerID, randomness, sectorIds)
}

func (*Prover) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return prover.Prover.GenerateSingleVanillaProof(ctx, replica, challenges)
}

func (*Prover) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	randomness[31] &= 0x3f
	return prover.Prover.GenerateWinningPoStWithVanilla(ctx, proofType, minerID, randomness, proofs)
}
