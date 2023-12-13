package posterv2

import (
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type Poster struct {
	runners []runner
}

type postDeps struct {
	chain          chain.API
	msg            messager.API
	rand           core.RandomnessAPI
	minerAPI       core.MinerAPI
	clock          clock.Clock
	prover         core.Prover
	verifier       core.Verifier
	sectorProving  core.SectorProving
	senderSelector core.SenderSelector
}
