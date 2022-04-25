package core

import (
	"context"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

type SectorManager interface {
	Allocate(ctx context.Context, spec AllocateSectorSpec) (*AllocatedSector, error)
}

type DealManager interface {
	Acquire(ctx context.Context, sid abi.SectorID, spec AcquireDealsSpec, job SectorWorkerJob) (Deals, error)
	Release(ctx context.Context, sid abi.SectorID, acquired Deals) error
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
	InitWith(ctx context.Context, sid abi.SectorID, proofType abi.RegisteredSealProof, fields ...interface{}) error
	Load(context.Context, abi.SectorID) (*SectorState, error)
	Update(context.Context, abi.SectorID, ...interface{}) error
	Finalize(context.Context, abi.SectorID, SectorStateChangeHook) error
	Restore(context.Context, abi.SectorID, SectorStateChangeHook) error
	All(ctx context.Context, ws SectorWorkerState, job SectorWorkerJob) ([]*SectorState, error)
	ForEach(ctx context.Context, ws SectorWorkerState, job SectorWorkerJob, fn func(SectorState) error) error
}

type SectorTypedIndexer interface {
	Find(ctx context.Context, sid abi.SectorID) (string, bool, error)
	Update(ctx context.Context, sid abi.SectorID, instance string) error
}

type SectorIndexer interface {
	Normal() SectorTypedIndexer
	Upgrade() SectorTypedIndexer
	StoreMgr() objstore.Manager
}

type SectorTracker interface {
	SinglePubToPrivateInfo(ctx context.Context, mid abi.ActorID, sectorInfo builtin.ExtendedSectorInfo, locator SectorLocator) (PrivateSectorInfo, error)
	SinglePrivateInfo(ctx context.Context, sref SectorRef, upgrade bool, locator SectorLocator) (PrivateSectorInfo, error)
	SingleProvable(ctx context.Context, sref SectorRef, upgrade bool, locator SectorLocator, strict bool) error
	Provable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error)
	PubToPrivate(ctx context.Context, mid abi.ActorID, sectorInfo []builtin.ExtendedSectorInfo, typer SectorPoStTyper) ([]FFIPrivateSectorInfo, error)
}

type SnapUpSectorManager interface {
	PreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (uint64, uint64, error)
	Candidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)
	Allocate(ctx context.Context, spec AllocateSectorSpec) (*SnapUpCandidate, error)
	Release(ctx context.Context, candidate *SnapUpCandidate) error
	Commit(ctx context.Context, sid abi.SectorID) error
}

type WorkerManager interface {
	Load(ctx context.Context, name string) (WorkerPingInfo, error)
	Update(ctx context.Context, winfo WorkerPingInfo) error
	All(ctx context.Context, filter func(*WorkerPingInfo) bool) ([]WorkerPingInfo, error)
}
