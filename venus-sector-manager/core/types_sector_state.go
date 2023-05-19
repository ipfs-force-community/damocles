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

type SectorFinalized bool
type SectorUpgraded bool
type SectorRemoved bool
type SectorImported bool
type SectorUpgradeLandedEpoch abi.ChainEpoch
type SectorUpgradeMessageID string
type SectorUpgradePublic SectorPublicInfo
type SectorNeedRebuild bool

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

	// Rebuild
	NeedRebuild SectorNeedRebuild
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

func (s *SectorState) PendingForSealingCommitment() bool {
	return s.MessageInfo.NeedSend && !bool(s.Upgraded) && !bool(s.Imported) && !bool(s.NeedRebuild)
}

func (s *SectorState) PendingForTerminateCommitment() bool {
	return s.TerminateInfo.AddedHeight > 0 && s.TerminateInfo.TerminatedAt == 0
}

func (s *SectorState) MatchWorkerJob(jtyp SectorWorkerJob) bool {
	switch jtyp {
	case SectorWorkerJobAll:
		return true

	case SectorWorkerJobSealing:
		return !bool(s.Upgraded) && !bool(s.NeedRebuild)

	case SectorWorkerJobSnapUp:
		return bool(s.Upgraded) && !bool(s.NeedRebuild)

	case SectorWorkerJobRebuild:
		return bool(s.NeedRebuild)

	default:
		return false

	}
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
	SectorWorkerJobRebuild SectorWorkerJob = 3
	SectorWorkerJobUnseal  SectorWorkerJob = 4
)
