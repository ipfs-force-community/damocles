package stage

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
)

const NameWindowPoSt = "window_post"

func ProofType2String(proofType abi.RegisteredPoStProof) string {
	switch proofType {

	case abi.RegisteredPoStProof_StackedDrgWinning2KiBV1:
		return "StackedDrgWinning2KiBV1"
	case abi.RegisteredPoStProof_StackedDrgWinning8MiBV1:
		return "StackedDrgWinning8MiBV1"
	case abi.RegisteredPoStProof_StackedDrgWinning512MiBV1:
		return "StackedDrgWinning512MiBV1"
	case abi.RegisteredPoStProof_StackedDrgWinning32GiBV1:
		return "StackedDrgWinning32GiBV1"
	case abi.RegisteredPoStProof_StackedDrgWinning64GiBV1:
		return "StackedDrgWinning64GiBV1"
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1:
		return "StackedDrgWindow2KiBV1"
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1:
		return "StackedDrgWindow8MiBV1"
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1:
		return "StackedDrgWindow512MiBV1"
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1:
		return "StackedDrgWindow32GiBV1"
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1:
		return "StackedDrgWindow64GiBV1"

	// rust-filecoin-proofs-api WindowPoSt uses api_version
	// V1_2 to fix the grindability issue, which we map here
	// as V1_1 for Venus/Lotus/actors compat reasons.
	// See: https://github.com/filecoin-project/filecoin-ffi/blob/cec06a79dc858f221f6542cff264b92b4f99c25d/rust/src/proofs/types.rs#L164-L173
	case abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1:
		return "StackedDrgWindow2KiBV1_2"
	case abi.RegisteredPoStProof_StackedDrgWindow8MiBV1_1:
		return "StackedDrgWindow8MiBV1_2"
	case abi.RegisteredPoStProof_StackedDrgWindow512MiBV1_1:
		return "StackedDrgWindow512MiBV1_2"
	case abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1:
		return "StackedDrgWindow32GiBV1_2"
	case abi.RegisteredPoStProof_StackedDrgWindow64GiBV1_1:
		return "StackedDrgWindow64GiBV1_2"

	default:
		return fmt.Sprintf("Unknown RegisteredPoStProof %d", proofType)
	}
}

type PoStReplicaInfo struct {
	SectorID   abi.SectorNumber `json:"sector_id"`
	CommR      [32]byte         `json:"comm_r"`
	CacheDir   string           `json:"cache_dir"`
	SealedFile string           `json:"sealed_file"`
}

type WindowPoStOutput struct {
	Proofs [][]byte           `json:"proofs"`
	Faults []abi.SectorNumber `json:"faults"`
}

type WindowPoSt struct {
	MinerID   abi.ActorID       `json:"miner_id"`
	ProofType string            `json:"proof_type"`
	Replicas  []PoStReplicaInfo `json:"replicas"`
	Seed      [32]byte          `json:"seed"`
}
