package prover

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

var _ core.Prover = Prover
var _ core.Verifier = Verifier

var Verifier verifier
var Prover prover

type (
	SortedPrivateSectorInfo = core.SortedPrivateSectorInfo
)

type ExtDoWindowPoStFunc func(stage.WindowPoSt) (stage.WindowPoStOutput, error)

func ExtGenerateWindowPoSt(minerID abi.ActorID, sectors SortedPrivateSectorInfo, randomness abi.PoStRandomness) func(ExtDoWindowPoStFunc) ([]builtin.PoStProof, []abi.SectorID, error) {
	randomness[31] &= 0x3f
	return func(doWork ExtDoWindowPoStFunc) ([]builtin.PoStProof, []abi.SectorID, error) {
		sectorInners := sectors.Values()
		if len(sectorInners) == 0 {
			return nil, nil, nil
		}

		// build stage.WindowPoSt
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

		output, err := doWork(data)
		if err != nil {
			return nil, nil, err
		}

		if faultCount := len(output.Faults); faultCount != 0 {
			faults := make([]abi.SectorID, faultCount)
			for fi := range output.Faults {
				faults[fi] = abi.SectorID{
					Miner:  minerID,
					Number: output.Faults[fi],
				}
			}

			return nil, faults, fmt.Errorf("got %d fault sectors", faultCount)
		}

		proofs := make([]builtin.PoStProof, len(output.Proofs))
		for pi := range output.Proofs {
			proofs[pi] = builtin.PoStProof{
				PoStProof:  proofType,
				ProofBytes: output.Proofs[pi],
			}
		}
		return proofs, nil, nil
	}
}
