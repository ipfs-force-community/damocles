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

func SectorSize2SealProofType(size abi.SectorSize) (abi.RegisteredSealProof, error) {
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
	SectorPathTypeCache       pathType = "cache"
	SectorPathTypeSealed      pathType = "sealed"
	SectorPathTypeUpdate      pathType = "update"
	SectorPathTypeUpdateCache pathType = "update-cache"
)

const sectorIDFormat = "s-t0%d-%d"

func SectorPath(typ pathType, sid abi.SectorID) string {
	return filepath.Join(string(typ), FormatSectorID(sid))
}

func FormatSectorID(sid abi.SectorID) string {
	return fmt.Sprintf(sectorIDFormat, sid.Miner, sid.Number)
}

func ScanSectorID(s string) (abi.SectorID, bool) {
	var sid abi.SectorID
	read, err := fmt.Sscanf(s, sectorIDFormat, &sid.Miner, &sid.Number)
	return sid, err == nil && read == 2
}

func CachedFilesForSectorSize(cacheDir string, ssize abi.SectorSize) []string {
	paths := []string{
		filepath.Join(cacheDir, "p_aux"),
		filepath.Join(cacheDir, "t_aux"),
	}
	switch ssize {
	case ss2KiB, ss8MiB, ss512MiB:
		paths = []string{filepath.Join(cacheDir, "sc-02-data-tree-r-last.dat")}
	case ss32GiB:
		for i := 0; i < 8; i++ {
			paths = append(paths, filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i)))
		}
	case ss64GiB:
		for i := 0; i < 16; i++ {
			paths = append(paths, filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i)))
		}
	default:
	}

	return paths
}
