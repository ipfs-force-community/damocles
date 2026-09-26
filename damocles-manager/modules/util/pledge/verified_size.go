package pledge

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
)

// VerifiedSectorSize reports the verified size the miner pledge API expects for
// a sector. FIP-0118 gives every sector maximum quality-adjusted power
// regardless of deal content, which that API expresses as a fully verified
// sector; before nv29 only pieces carrying an allocation count.
func VerifiedSectorSize(nv network.Version, ssize abi.SectorSize, sumVerified uint64) uint64 {
	if nv >= network.Version29 {
		return uint64(ssize)
	}

	return sumVerified
}
