package api

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
)

type RandomnessAPI interface {
	GetTicket(context.Context) (Ticket, error)
	GetSeed(context.Context) (Seed, error)
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
