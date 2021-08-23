package api

import (
	"context"

	"github.com/filecoin-project/venus/pkg/types"

	"github.com/filecoin-project/go-state-types/abi"
)

type SealerAPI interface {
	AllocateSector(context.Context, AllocateSectorSpec) (*AllocatedSector, error)

	AcquireDeals(context.Context, abi.SectorID, AcquireDealsSpec) (Deals, error)

	AssignTicket(context.Context, abi.SectorID) (Ticket, error)

	SubmitPreCommit(context.Context, AllocatedSector, PreCommitOnChainInfo, bool) (SubmitPreCommitResp, error)

	PollPreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	WaitSeed(context.Context, abi.SectorID) (WaitSeedResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofOnChainInfo, bool) (SubmitProofResp, error)

	PollProofState(context.Context, abi.SectorID) (PollProofStateResp, error)

	ReportState(context.Context, abi.SectorID, ReportStateReq) error

	ReportFinalized(context.Context, abi.SectorID) error
}

type RandomnessAPI interface {
	GetTicket(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Ticket, error)
	GetSeed(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Seed, error)
}

type MinerInfoAPI interface {
	Get(context.Context, abi.ActorID) (*MinerInfo, error)
}

type SectorManager interface {
	Allocate(context.Context, []abi.ActorID, []abi.RegisteredSealProof) (*AllocatedSector, error)
}

type DealManager interface {
	Acquire(context.Context, abi.SectorID, *uint) (Deals, error)
	Release(context.Context, Deals) error
}

type CommitmentManager interface {
	SubmitPreCommit(context.Context, abi.SectorID, PreCommitInfo, bool) (SubmitPreCommitResp, error)
	PreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofInfo, bool) (SubmitProofResp, error)
	ProofState(context.Context, abi.SectorID) (PollProofStateResp, error)
}

type SectorNumberAllocator interface {
	Next(context.Context, abi.ActorID) (uint64, error)
}

type SectorStateManager interface {
	Init(context.Context, abi.SectorID, abi.RegisteredSealProof) error
	Load(context.Context, abi.SectorID) (*SectorState, error)
	Update(context.Context, abi.SectorID, ...interface{}) error
	Finalize(context.Context, abi.SectorID) error
	All(ctx context.Context) ([]*SectorState, error)
}
