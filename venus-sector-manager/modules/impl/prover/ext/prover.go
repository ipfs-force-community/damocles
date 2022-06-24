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
)

var _ core.Prover = (*Prover)(nil)

func New(ctx context.Context, cfgs []extproc.ExtProcessorConfig) (*Prover, error) {
	proc, err := extproc.New(ctx, extproc.StageNameWindowPoSt, cfgs)
	if err != nil {
		return nil, fmt.Errorf("construct Processor: %w", err)
	}

	return &Prover{
		proc: proc,
	}, nil
}

type Prover struct {
	proc *extproc.Processor
}

func (p *Prover) Run() {
	p.proc.Run()
}

func (p *Prover) Close() {
	p.proc.Close()
}

func (*Prover) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return prover.Prover.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *Prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, []abi.SectorID, error) {
	sectorInners := sectors.Values()
	if len(sectorInners) == 0 {
		return nil, nil, nil
	}

	proofType := sectorInners[0].PoStProofType
	data := extproc.WindowPoSt{
		MinerID:   minerID,
		ProofType: extproc.ProofType2String(proofType),
	}
	copy(data.Seed[:], randomness[:])

	for i := range sectorInners {
		inner := sectorInners[i]

		if pt := inner.PoStProofType; pt != proofType {
			return nil, nil, fmt.Errorf("proof type not match for sector %d of miner %d: want %s, got %s", inner.SectorNumber, minerID, extproc.ProofType2String(proofType), extproc.ProofType2String(pt))
		}

		commR, err := util.CID2ReplicaCommitment(inner.SealedCID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid selaed cid %s for sector %d of miner %d: %w", inner.SealedCID, inner.SectorNumber, minerID, err)
		}

		data.Replicas = append(data.Replicas, extproc.WindowPoStReplicaInfo{
			SectorID:   inner.SectorNumber,
			CommR:      commR,
			CacheDir:   inner.CacheDirPath,
			SealedFile: inner.SealedSectorPath,
		})
	}

	var res extproc.WindowPoStOutput

	err := p.proc.Process(ctx, data, &res)
	if err != nil {
		return nil, nil, fmt.Errorf("Processor.Process: %w", err)
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

func (*Prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return prover.Prover.GenerateWinningPoSt(ctx, minerID, sectors, randomness)
}

func (*Prover) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return prover.Prover.GeneratePoStFallbackSectorChallenges(ctx, proofType, minerID, randomness, sectorIds)
}

func (*Prover) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return prover.Prover.GenerateSingleVanillaProof(ctx, replica, challenges)
}

func (*Prover) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	return prover.Prover.GenerateWinningPoStWithVanilla(ctx, proofType, minerID, randomness, proofs)
}
