package api

import (
	"github.com/filecoin-project/go-state-types/abi"
)

type SectorState struct {
	ID         abi.SectorID
	SectorType abi.RegisteredSealProof

	// may be nil
	Deals  Deals
	Ticket *Ticket
	Seed   *Seed
	Pre    *PreCommitOnChainInfo
	Proof  *ProofOnChainInfo

	MessageInfo *MessageInfo
}

func (s SectorState) DealIDs() []abi.DealID {
	res := make([]abi.DealID, len(s.Deals))
	for i := range s.Deals {
		res[i] = s.Deals[i].ID
	}
	return res
}
