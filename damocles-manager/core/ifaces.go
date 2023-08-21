package core

import (
	"context"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
)

type SectorManager interface {
	Allocate(ctx context.Context, spec AllocateSectorSpec, count uint32) ([]*AllocatedSector, error)
}

type DealManager interface {
	Acquire(ctx context.Context, sid abi.SectorID, spec AcquireDealsSpec, lifetime *AcquireDealsLifetime, job SectorWorkerJob) (Deals, error)
	Release(ctx context.Context, sid abi.SectorID, acquired Deals) error
}

type CommitmentManager interface {
	SubmitPreCommit(context.Context, abi.SectorID, PreCommitInfo, bool) (SubmitPreCommitResp, error)
	PreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)
	// PreCommitStateBatch(ctx context.Context, minerID abi.ActorID, numbers []abi.SectorNumber) (map[abi.SectorNumber]PollPreCommitStateResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofInfo, bool) (SubmitProofResp, error)
	ProofState(context.Context, abi.SectorID) (PollProofStateResp, error)

	SubmitTerminate(context.Context, abi.SectorID) (SubmitTerminateResp, error)
	TerminateState(context.Context, abi.SectorID) (TerminateInfo, error)
}

type SectorNumberAllocator interface {
	NextN(ctx context.Context, mid abi.ActorID, n uint32, minNumber uint64, check func(uint64) bool) (uint64, bool, error)
}

type SectorStateManager interface {
	Import(ctx context.Context, ws SectorWorkerState, state *SectorState, override bool) (bool, error)
	Init(ctx context.Context, sectors []*AllocatedSector, ws SectorWorkerState) error
	InitWith(ctx context.Context, sectors []*AllocatedSector, ws SectorWorkerState, fields ...interface{}) error
	Load(ctx context.Context, sid abi.SectorID, ws SectorWorkerState) (*SectorState, error)
	Update(ctx context.Context, sid abi.SectorID, ws SectorWorkerState, fieldvals ...interface{}) error
	Finalize(context.Context, abi.SectorID, SectorStateChangeHook) error
	Restore(context.Context, abi.SectorID, SectorStateChangeHook) error
	All(ctx context.Context, ws SectorWorkerState, job SectorWorkerJob) ([]*SectorState, error)
	ForEach(ctx context.Context, ws SectorWorkerState, job SectorWorkerJob, fn func(SectorState) error) error
}

type SectorTypedIndexer interface {
	Find(ctx context.Context, sid abi.SectorID) (SectorAccessStores, bool, error)
	Update(ctx context.Context, sid abi.SectorID, stores SectorAccessStores) error
}

type SectorIndexer interface {
	Normal() SectorTypedIndexer
	Upgrade() SectorTypedIndexer
	StoreMgr() objstore.Manager
}

type SectorTracker interface {
	SinglePubToPrivateInfo(ctx context.Context, mid abi.ActorID, sectorInfo builtin.ExtendedSectorInfo, locator SectorLocator) (PrivateSectorInfo, error)
	SinglePrivateInfo(ctx context.Context, sref SectorRef, upgrade bool, locator SectorLocator) (PrivateSectorInfo, error)
	PubToPrivate(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectorInfo []builtin.ExtendedSectorInfo) ([]FFIPrivateSectorInfo, error)
}

type SectorProving interface {
	SingleProvable(ctx context.Context, postProofType abi.RegisteredPoStProof, sref SectorRef, upgrade bool, locator SectorLocator, strict, stateCheck bool) error
	Provable(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict, stateCheck bool) (map[abi.SectorNumber]string, error)
	SectorTracker
}

type SnapUpSectorManager interface {
	PreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (uint64, uint64, error)
	Candidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)
	Allocate(ctx context.Context, spec AllocateSectorSpec) (*SnapUpCandidate, error)
	Release(ctx context.Context, candidate *SnapUpCandidate) error
	Commit(ctx context.Context, sid abi.SectorID) error
	CancelCommitment(ctx context.Context, sid abi.SectorID)
}

type WorkerManager interface {
	Load(ctx context.Context, name string) (WorkerPingInfo, error)
	Update(ctx context.Context, winfo WorkerPingInfo) error
	All(ctx context.Context, filter func(*WorkerPingInfo) bool) ([]WorkerPingInfo, error)
	Remove(ctx context.Context, name string) error
}

type RebuildSectorManager interface {
	Set(ctx context.Context, sid abi.SectorID, info SectorRebuildInfo) error
	Allocate(ctx context.Context, spec AllocateSectorSpec) (*SectorRebuildInfo, error)
}

type UnsealSectorManager interface {
	SetAndCheck(ctx context.Context, req *SectorUnsealInfo) (gtypes.UnsealState, error)
	Allocate(ctx context.Context, spec AllocateSectorSpec) (*SectorUnsealInfo, error)
	Achieve(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, unsealErr string) error
	OnAchieve(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, hook func())
	AcquireDest(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid) ([]string, error)
}
