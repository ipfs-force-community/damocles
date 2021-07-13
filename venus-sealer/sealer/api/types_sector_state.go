package api

import "github.com/filecoin-project/go-state-types/abi"

type SectorState struct {
	ID     abi.SectorID
	Deals  Deals
	Ticket Ticket
	Seed   Seed
	Pre    PreCommitOnChainInfo
	Proof  ProofOnChainInfo
}
