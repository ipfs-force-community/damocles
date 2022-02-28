package api

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

const MajorVersion = 0

var Empty Meta

type Meta *struct{}

type SealerAPI interface {
	AllocateSector(context.Context, AllocateSectorSpec) (*AllocatedSector, error)

	AcquireDeals(context.Context, abi.SectorID, AcquireDealsSpec) (Deals, error)

	AssignTicket(context.Context, abi.SectorID) (Ticket, error)

	SubmitPreCommit(context.Context, AllocatedSector, PreCommitOnChainInfo, bool) (SubmitPreCommitResp, error)

	PollPreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	SubmitPersisted(context.Context, abi.SectorID, string) (bool, error)

	WaitSeed(context.Context, abi.SectorID) (WaitSeedResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofOnChainInfo, bool) (SubmitProofResp, error)

	PollProofState(context.Context, abi.SectorID) (PollProofStateResp, error)

	ListSectors(context.Context) ([]*SectorState, error)

	ReportState(context.Context, abi.SectorID, ReportStateReq) (Meta, error)

	ReportFinalized(context.Context, abi.SectorID) (Meta, error)

	ReportAborted(context.Context, abi.SectorID, string) (Meta, error)
}

type RandomnessAPI interface {
	GetTicket(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Ticket, error)
	GetSeed(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Seed, error)
	GetWindowPoStChanlleengeRand(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (WindowPoStRandomness, error)
	GetWindowPoStCommitRand(context.Context, types.TipSetKey, abi.ChainEpoch) (WindowPoStRandomness, error)
}

type MinerInfoAPI interface {
	Get(context.Context, abi.ActorID) (*MinerInfo, error)
}

type SectorManager interface {
	Allocate(context.Context, []abi.ActorID, []abi.RegisteredSealProof) (*AllocatedSector, error)
}

type DealManager interface {
	Acquire(context.Context, abi.SectorID, *uint) (Deals, error)
	Release(context.Context, abi.SectorID, Deals) error
}

type CommitmentManager interface {
	SubmitPreCommit(context.Context, abi.SectorID, PreCommitInfo, bool) (SubmitPreCommitResp, error)
	PreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofInfo, bool) (SubmitProofResp, error)
	ProofState(context.Context, abi.SectorID) (PollProofStateResp, error)
}

type SectorNumberAllocator interface {
	Next(context.Context, abi.ActorID, uint64, func(uint64) bool) (uint64, bool, error)
}

type SectorStateManager interface {
	Init(context.Context, abi.SectorID, abi.RegisteredSealProof) error
	Load(context.Context, abi.SectorID) (*SectorState, error)
	Update(context.Context, abi.SectorID, ...interface{}) error
	Finalize(context.Context, abi.SectorID, func(*SectorState) error) error
	All(ctx context.Context) ([]*SectorState, error)
}

type SectorIndexer interface {
	Find(context.Context, abi.SectorID) (string, bool, error)
	Update(context.Context, abi.SectorID, string) error
	StoreMgr() objstore.Manager
}
