package core

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
)

// applies outside changes to the state, returns if it is ok to go on, and any error if exist
type SectorStateChangeHook func(st *SectorState) (bool, error)

// returns the persist instance name, existence
type SectorLocator func(ctx context.Context, sid abi.SectorID) (SectorAccessStores, bool, error)

type (
	SectorFinalized          bool
	SectorUpgraded           bool
	SectorRemoved            bool
	SectorImported           bool
	SectorUpgradeLandedEpoch abi.ChainEpoch
	SectorUpgradeMessageID   string
	SectorUpgradePublic      SectorPublicInfo
	SectorNeedRebuild        bool
	SectorUnsealing          bool
)

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
	Ticket       *Ticket        `json:",omitempty"`
	Seed         *Seed          `json:",omitempty"`
	LegacyPieces Deals          `json:"Deals"`
	Pieces       SectorPieces   `json:"Pieces"`
	Pre          *PreCommitInfo `json:",omitempty"`
	Proof        *ProofInfo     `json:",omitempty"`

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

	// Unseal
	Unsealing SectorUnsealing
}

// TODO: we need iter
func (s *SectorState) SectorPiece() []SectorPiece {
	c := len(s.Pieces)
	if c == 0 {
		c = len(s.LegacyPieces)
	}
	sp := make([]SectorPiece, 0, c)

	if len(s.Pieces) > 0 {
		for _, p := range s.Pieces {
			sp = append(sp, p)
		}
	} else if len(s.LegacyPieces) > 0 {
		for _, p := range s.LegacyPieces {
			sp = append(sp, p)
		}
	}
	return sp
}

func (s *SectorState) PieceInfos() []PieceInfo {
	return lo.Map(s.SectorPiece(), func(p SectorPiece, _i int) PieceInfo {
		return p.PieceInfo()
	})
}

func (s *SectorState) HasDDODeal() bool {
	for _, piece := range s.SectorPiece() {
		if !piece.IsBuiltinMarket() {
			return true
		}
	}
	return false
}

func (s *SectorState) HasBuiltinMarketDeal() bool {
	for _, piece := range s.SectorPiece() {
		if piece.IsBuiltinMarket() {
			return true
		}
	}
	return false
}

func (s *SectorState) DealIDs() []abi.DealID {
	return lo.FilterMap(s.SectorPiece(), func(p SectorPiece, _i int) (abi.DealID, bool) {
		dealID := p.DealID()
		if dealID == abi.DealID(0) {
			return abi.DealID(0), false
		}
		return dealID, true
	})
}

func (s *SectorState) HasData() bool {
	for _, piece := range s.Pieces {
		if piece.HasDealInfo() {
			return true
		}
	}

	for _, piece := range s.LegacyPieces {
		if piece.HasDealInfo() {
			return true
		}
	}

	return false
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
		return !bool(s.Upgraded) && !bool(s.NeedRebuild) && !bool(s.Unsealing)

	case SectorWorkerJobSnapUp:
		return bool(s.Upgraded) && !bool(s.NeedRebuild) && !bool(s.Unsealing)

	case SectorWorkerJobRebuild:
		return bool(s.NeedRebuild)

	case SectorWorkerJobUnseal:
		return bool(s.Unsealing)

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
