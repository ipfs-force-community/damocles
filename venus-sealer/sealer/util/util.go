package util

import (
	"fmt"
	"path/filepath"

	"github.com/filecoin-project/go-state-types/abi"
)

const (
	ss2KiB   = 2 << 10
	ss8MiB   = 8 << 20
	ss512MiB = 512 << 20
	ss32GiB  = 32 << 30
	ss64GiB  = 64 << 30
)

var ErrInvalidSectorSize = fmt.Errorf("invalid sector size")

func SectorSize2SealProofType(size uint64) (abi.RegisteredSealProof, error) {
	switch size {
	case ss2KiB:
		return abi.RegisteredSealProof_StackedDrg2KiBV1_1, nil

	case ss8MiB:
		return abi.RegisteredSealProof_StackedDrg8MiBV1_1, nil

	case ss512MiB:
		return abi.RegisteredSealProof_StackedDrg512MiBV1_1, nil

	case ss32GiB:
		return abi.RegisteredSealProof_StackedDrg32GiBV1_1, nil

	case ss64GiB:
		return abi.RegisteredSealProof_StackedDrg64GiBV1_1, nil

	default:
		return 0, fmt.Errorf("%w: %d", ErrInvalidSectorSize, size)
	}
}

type pathType string

const (
	SectorPathTypeCache    = "cache"
	SectorPathTypeSealed   = "sealed"
	SectorPathTypeUnsealed = "unsealed"
)

func SectorPath(typ pathType, sid abi.SectorID) string {
	return filepath.Join(string(typ), fmt.Sprintf("s-%d-%d", sid.Miner, sid.Number))
}
