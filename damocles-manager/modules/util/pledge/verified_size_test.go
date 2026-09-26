package pledge

import (
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/network"
)

func TestVerifiedSectorSize(t *testing.T) {
	const ssize abi.SectorSize = 32 << 30
	const partial = uint64(1) << 30

	for _, tc := range []struct {
		name        string
		nv          network.Version
		sumVerified uint64
		want        uint64
	}{
		{"nv29 without allocations", network.Version29, 0, uint64(ssize)},
		{"nv29 with partial allocations", network.Version29, partial, uint64(ssize)},
		{"pre-nv29 without allocations", network.Version28, 0, 0},
		{"pre-nv29 with partial allocations", network.Version28, partial, partial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifiedSectorSize(tc.nv, ssize, tc.sumVerified); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestVerifiedSectorSize_QualityAdjustedPower(t *testing.T) {
	const ssize abi.SectorSize = 32 << 30
	multiplier := uint64(builtin.QualityBaseMultiplier.Int64())

	// A nv29 sector whose pieces carry no allocation key, which is every
	// builtin-market deal on nv29.
	var sumVerified uint64

	// Summing allocations reports no verified data, so all 32 GiB would be
	// pledged as unverified, at 1x quality-adjusted power.
	unverifiedQAPower := uint64(ssize)

	verifiedSize := VerifiedSectorSize(network.Version29, ssize, sumVerified)
	verifiedQAPower := uint64(ssize-abi.SectorSize(verifiedSize)) + verifiedSize*multiplier

	if verifiedSize != uint64(ssize) {
		t.Fatalf("verifiedSize = %d, want %d", verifiedSize, uint64(ssize))
	}
	if verifiedQAPower != unverifiedQAPower*multiplier {
		t.Errorf("quality-adjusted power = %d, want %d", verifiedQAPower, unverifiedQAPower*multiplier)
	}

	t.Logf("verifiedSize %d -> %d; quality-adjusted power %d -> %d",
		sumVerified, verifiedSize, unverifiedQAPower, verifiedQAPower)
}
