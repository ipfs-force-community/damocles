package prover

import (
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

var _ core.Prover = Prover
var _ core.Verifier = Verifier

var Verifier verifier
var Prover prover

type (
	SortedPrivateSectorInfo = core.SortedPrivateSectorInfo
)
