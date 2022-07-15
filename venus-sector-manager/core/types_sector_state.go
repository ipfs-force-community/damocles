package core

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
)

// applies outside changes to the state, returns if it is ok to go on, and any error if exist
type SectorStateChangeHook func(st *SectorState) (bool, error)

// returns the persist instance name, existence
type SectorLocator func(ctx context.Context, sid abi.SectorID) (SectorAccessStores, bool, error)

type SectorPoStTyper func(proofType abi.RegisteredSealProof) (abi.RegisteredPoStProof, error)

func SectorWindowPoSt(proofType abi.RegisteredSealProof) (abi.RegisteredPoStProof, error) {
	return proofType.RegisteredWindowPoStProof()
}

func SectorWinningPoSt(proofType abi.RegisteredSealProof) (abi.RegisteredPoStProof, error) {
	return proofType.RegisteredWinningPoStProof()
}

type SectorFinalized bool
type SectorUpgraded bool
type SectorRemoved bool
type SectorImported bool
type SectorUpgradeLandedEpoch abi.ChainEpoch
type SectorUpgradeMessageID string
type SectorUpgradePublic SectorPublicInfo

type SectorUpgradedInfo struct {
	AccessInstance string
	SealedCID      cid.Cid
	UnsealedCID    cid.Cid
	Proof          []byte
}

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

	TerminateInfo TerminateInfo

	LatestState *ReportStateReq `json:",omitempty"`
	Finalized   SectorFinalized
	Removed     SectorRemoved
	AbortReason string

	// for snapup
	Upgraded           SectorUpgraded
	UpgradePublic      *SectorUpgradePublic
	UpgradedInfo       *SectorUpgradedInfo
	UpgradeMessageID   *SectorUpgradeMessageID
	UpgradeLandedEpoch *SectorUpgradeLandedEpoch

	// Imported
	Imported SectorImported
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

type SectorWorkerJob int

const (
	SectorWorkerJobAll     SectorWorkerJob = 0
	SectorWorkerJobSealing SectorWorkerJob = 1
	SectorWorkerJobSnapUp  SectorWorkerJob = 2
)
