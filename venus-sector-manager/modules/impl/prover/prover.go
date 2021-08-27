package prover

import (
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

var _ api.Prover = Prover
var _ api.Verifier = Verifier

var Verifier verifier
var Prover prover

type (
	SortedPrivateSectorInfo = api.SortedPrivateSectorInfo
)
