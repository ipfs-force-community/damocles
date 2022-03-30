package api

import (
	"github.com/filecoin-project/go-state-types/abi"
)

type Finalized bool

type SectorState struct {
	ID         abi.SectorID
	SectorType abi.RegisteredSealProof

	// may be nil
	Deals  Deals
	Ticket *Ticket
	Seed   *Seed
	Pre    *PreCommitInfo
	Proof  *ProofInfo

	MessageInfo MessageInfo

	LatestState *ReportStateReq
	Finalized   Finalized
	AbortReason string
}

func (s SectorState) DealIDs() []abi.DealID {
	res := make([]abi.DealID, 0, len(s.Deals))
	for i := range s.Deals {
		if id := s.Deals[i].ID; id != 0 {
			res = append(res, id)
		}
	}
	return res
}

type SectorWorkerState string

const (
	WorkerOnline  SectorWorkerState = "online"
	WorkerOffline SectorWorkerState = "offline"
)
