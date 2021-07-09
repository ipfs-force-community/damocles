package api

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
)

type SealerAPI interface {
	AllocateSector(context.Context, AllocateSectorSpec) (*AllocatedSector, error)

	AcquireDeals(context.Context, abi.SectorID, AcquireDealsSpec) (Deals, error)

	AssignTicket(context.Context, abi.SectorID) (Ticket, error)

	SubmitPreCommit(context.Context, AllocatedSector, PreCommitOnChainInfo) (SubmitPreCommitResp, error)

	PollPreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	AssignSeed(context.Context, abi.SectorID) (Seed, error)

	SubmitProof(context.Context, abi.SectorID, ProofOnChainInfo) (SubmitProofResp, error)

	PollProofState(context.Context, abi.SectorID) (PollProofStateResp, error)
}

type RandomnessAPI interface {
	GetTicket(context.Context) (Ticket, error)
	GetSeed(context.Context) (Seed, error)
}

type MinerInfoAPI interface {
	Get(context.Context, abi.ActorID) (*MinerInfo, error)
}

type SectorManager interface {
	Allocate(context.Context, []abi.ActorID, []abi.RegisteredSealProof) (*AllocatedSector, error)
}

type DealManager interface {
	Acquire(context.Context, abi.SectorID, *uint) (Deals, error)
}

type CommitmentManager interface {
	SubmitPreCommit(context.Context, abi.SectorID, PreCommitOnChainInfo) (SubmitPreCommitResp, error)
	PreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofOnChainInfo) (SubmitProofResp, error)
	ProofState(context.Context, abi.SectorID) (PollProofStateResp, error)
}

type SectorNumberAllocator interface {
	Next(context.Context, abi.ActorID) (uint64, error)
}

type SectorStateManager interface {
	Update(context.Context, abi.SectorID, ...interface{}) error
}
