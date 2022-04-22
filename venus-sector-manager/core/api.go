package core

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
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

	ReportState(context.Context, abi.SectorID, ReportStateReq) (Meta, error)

	ReportFinalized(context.Context, abi.SectorID) (Meta, error)

	ReportAborted(context.Context, abi.SectorID, string) (Meta, error)

	// Snap
	AllocateSanpUpSector(ctx context.Context, spec AllocateSnapUpSpec) (*AllocatedSnapUpSector, error)

	SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo SnapUpOnChainInfo) (SubmitSnapUpProofResp, error)

	// utils
	SealerCliAPI
}

type SealerCliAPI interface {
	ListSectors(context.Context, SectorWorkerState) ([]*SectorState, error)

	RestoreSector(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error)

	CheckProvable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error)

	SimulateWdPoSt(context.Context, address.Address, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error

	SnapUpPreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*SnapUpFetchResult, error)

	SnapUpCandidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)

	ProvingSectorInfo(ctx context.Context, sid abi.SectorID) (ProvingSectorInfo, error)
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
