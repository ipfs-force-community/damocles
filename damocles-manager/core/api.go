package core

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

//go:generate go run gen.go -interface=SealerAPI,SealerCliAPI,RandomnessAPI,MinerAPI,WorkerWdPoStAPI

const (
	// TODO: The APINamespace is Venus due to historical reasons,
	// and we should consider changing it to a more appropriate name in future versions
	APINamespace = "Venus"
	MajorVersion = 0
)

var Empty Meta

type Meta *struct{}

type API interface {
	SealerAPI
	SealerCliAPI
	RandomnessAPI
	MinerAPI
	WorkerWdPoStAPI
}

type SealerAPI interface {
	AllocateSector(context.Context, AllocateSectorSpec) (*AllocatedSector, error)

	AcquireDeals(context.Context, abi.SectorID, AcquireDealsSpec) (Deals, error)

	AssignTicket(context.Context, abi.SectorID) (Ticket, error)

	SubmitPreCommit(context.Context, AllocatedSector, PreCommitOnChainInfo, bool) (SubmitPreCommitResp, error)

	PollPreCommitState(context.Context, abi.SectorID) (PollPreCommitStateResp, error)

	// TODO：维持兼容性，应当在将来移除
	SubmitPersisted(context.Context, abi.SectorID, string) (bool, error)

	SubmitPersistedEx(ctx context.Context, sid abi.SectorID, instanceName string, isUpgrade bool) (bool, error)

	WaitSeed(context.Context, abi.SectorID) (WaitSeedResp, error)

	SubmitProof(context.Context, abi.SectorID, ProofOnChainInfo, bool) (SubmitProofResp, error)

	PollProofState(context.Context, abi.SectorID) (PollProofStateResp, error)

	ReportState(context.Context, abi.SectorID, ReportStateReq) (Meta, error)

	ReportFinalized(context.Context, abi.SectorID) (Meta, error)

	ReportAborted(context.Context, abi.SectorID, string) (Meta, error)

	// Snap
	AllocateSanpUpSector(ctx context.Context, spec AllocateSnapUpSpec) (*AllocatedSnapUpSector, error)

	SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo SnapUpOnChainInfo) (SubmitSnapUpProofResp, error)

	// Rebuild
	AllocateRebuildSector(ctx context.Context, spec AllocateSectorSpec) (*SectorRebuildInfo, error)

	// Workers
	WorkerPing(ctx context.Context, winfo WorkerInfo) (Meta, error)

	// Store
	StoreReserveSpace(ctx context.Context, sid abi.SectorID, size uint64, candidates []string) (*StoreBasicInfo, error)

	StoreBasicInfo(ctx context.Context, instanceName string) (*StoreBasicInfo, error)

	// Unseal Sector
	AllocateUnsealSector(ctx context.Context, spec AllocateSectorSpec) (*SectorUnsealInfo, error)
	AchieveUnsealSector(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, errInfo string) (Meta, error)
	AcquireUnsealDest(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid) ([]string, error)
}

type SealerCliAPI interface {
	ListSectors(context.Context, SectorWorkerState, SectorWorkerJob) ([]*SectorState, error)

	FindSector(ctx context.Context, state SectorWorkerState, sid abi.SectorID) (*SectorState, error)

	FindSectorInAllStates(ctx context.Context, sid abi.SectorID) (*SectorState, error)

	FindSectorsWithDeal(ctx context.Context, state SectorWorkerState, dealID abi.DealID) ([]*SectorState, error)

	FindSectorWithPiece(ctx context.Context, state SectorWorkerState, pieceCid cid.Cid) (*SectorState, error)

	ImportSector(ctx context.Context, ws SectorWorkerState, state *SectorState, override bool) (bool, error)

	RestoreSector(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error)

	CheckProvable(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict, stateCheck bool) (map[abi.SectorNumber]string, error)

	SimulateWdPoSt(context.Context, uint64, address.Address, abi.RegisteredPoStProof, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error

	SnapUpPreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*SnapUpFetchResult, error)

	SnapUpCandidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)

	SnapUpCancelCommitment(ctx context.Context, sid abi.SectorID) error

	ProvingSectorInfo(ctx context.Context, sid abi.SectorID) (ProvingSectorInfo, error)

	WorkerGetPingInfo(ctx context.Context, name string) (*WorkerPingInfo, error)

	WorkerPingInfoList(ctx context.Context) ([]WorkerPingInfo, error)

	WorkerPingInfoRemove(ctx context.Context, name string) error

	SectorIndexerFind(ctx context.Context, indexType SectorIndexType, sid abi.SectorID) (SectorIndexLocation, error)

	TerminateSector(context.Context, abi.SectorID) (SubmitTerminateResp, error)

	PollTerminateSectorState(context.Context, abi.SectorID) (TerminateInfo, error)

	RemoveSector(context.Context, abi.SectorID) error

	FinalizeSector(context.Context, abi.SectorID) error

	StoreReleaseReserved(ctx context.Context, sid abi.SectorID) (bool, error)

	StoreList(ctx context.Context) ([]StoreDetailedInfo, error)

	SectorSetForRebuild(ctx context.Context, sid abi.SectorID, opt RebuildOptions) (bool, error)

	// Unseal Sector
	UnsealPiece(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, offset types.UnpaddedByteIndex, size abi.UnpaddedPieceSize, dest string) (<-chan []byte, error)

	Version(ctx context.Context) (string, error)
}

type RandomnessAPI interface {
	GetTicket(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Ticket, error)
	GetSeed(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (Seed, error)
	GetWindowPoStChanlleengeRand(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (WindowPoStRandomness, error)
	GetWindowPoStCommitRand(context.Context, types.TipSetKey, abi.ChainEpoch) (WindowPoStRandomness, error)
}

type MinerAPI interface {
	GetInfo(context.Context, abi.ActorID) (*MinerInfo, error)
	GetMinerConfig(context.Context, abi.ActorID) (*modules.MinerConfig, error)
}

type WorkerWdPoStAPI interface {
	WdPoStHeartbeatTasks(ctx context.Context, runningTaskIDs []string, workerName string) (Meta, error)
	WdPoStAllocateTasks(ctx context.Context, spec AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []*WdPoStAllocatedTask, err error)
	WdPoStFinishTask(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) (Meta, error)
	WdPoStResetTask(ctx context.Context, taskID string) (Meta, error)
	WdPoStRemoveTask(ctx context.Context, taskID string) (Meta, error)
	WdPoStAllTasks(ctx context.Context) ([]*WdPoStTask, error)
}
