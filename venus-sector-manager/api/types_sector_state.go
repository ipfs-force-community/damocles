package api

import (
	"github.com/filecoin-project/go-state-types/abi"
)

type Finalized bool

type SectorState struct {
	ID         abi.SectorID
	SectorType abi.RegisteredSealProof

	// may be nil
	Ticket *Ticket        `json:",omitempty"`
	Seed   *Seed          `json:",omitempty"`
	Pieces Deals          `json:"Deals"`
	Pre    *PreCommitInfo `json:",omitempty"`
	Proof  *ProofInfo     `json:",omitempty"`

	MessageInfo MessageInfo

	LatestState *ReportStateReq `json:",omitempty"`
	Finalized   Finalized
	AbortReason string
}

func (s SectorState) DealIDs() []abi.DealID {
	res := make([]abi.DealID, 0, len(s.Pieces))
	for i := range s.Pieces {
		if id := s.Pieces[i].ID; id != 0 {
			res = append(res, id)
		}
	}
	return res
}

func (s SectorState) Deals() Deals {
	res := make([]DealInfo, 0)
	for _, piece := range s.Pieces {
		if piece.ID != 0 {
			res = append(res, piece)
		}
	}
	return res
}

type SectorWorkerState string

const (
	WorkerOnline  SectorWorkerState = "online"
	WorkerOffline SectorWorkerState = "offline"
)
