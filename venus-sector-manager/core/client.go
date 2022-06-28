package core

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
)

var UnavailableSealerCliClient = SealerCliClient{
	ListSectors: func(context.Context, SectorWorkerState, SectorWorkerJob) ([]*SectorState, error) {
		panic("sealer client unavailable")
	},

	RestoreSector: func(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error) {
		panic("sealer client unavailable")
	},

	ReportFinalized: func(context.Context, abi.SectorID) (Meta, error) { panic("sealer client unavailable") },

	ReportAborted: func(context.Context, abi.SectorID, string) (Meta, error) { panic("sealer client unavailable") },

	CheckProvable: func(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
		panic("sealer client unavailable")
	},

	SimulateWdPoSt: func(context.Context, address.Address, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error {
		panic("sealer client unavailable")
	},

	SnapUpPreFetch: func(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*SnapUpFetchResult, error) {
		panic("sealer client unavailable")
	},

	SnapUpCandidates: func(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error) {
		panic("sealer client unavailable")
	},

	SnapUpCancelCommitment: func(ctx context.Context, sid abi.SectorID) error {
		panic("sealer client unavailable")
	},

	ProvingSectorInfo: func(ctx context.Context, sid abi.SectorID) (ProvingSectorInfo, error) {
		panic("sealer client unavailable")
	},

	WorkerGetPingInfo: func(ctx context.Context, name string) (*WorkerPingInfo, error) {
		panic("sealer client unavailable")
	},

	WorkerPingInfoList: func(ctx context.Context) ([]WorkerPingInfo, error) {
		panic("sealer client unavailable")
	},

	SectorIndexerFind: func(ctx context.Context, indexType SectorIndexType, sid abi.SectorID) (SectorIndexLocation, error) {
		panic("sealer client unavailable")
	},

	TerminateSector: func(context.Context, abi.SectorID) (SubmitTerminateResp, error) {
		panic("sealer client unavailable")
	},

	PollTerminateSectorState: func(context.Context, abi.SectorID) (TerminateInfo, error) {
		panic("sealer client unavailable")
	},

	RemoveSector: func(context.Context, abi.SectorID) error {
		panic("sealer client unavailable")
	},

	StoreReleaseReserved: func(ctx context.Context, sid abi.SectorID) (bool, error) {
		panic("sealer client unavailable")
	},

	StoreList: func(ctx context.Context) ([]StoreDetailedInfo, error) {
		panic("sealer client unavailable")
	},
}

type SealerCliClient struct {
	ListSectors func(context.Context, SectorWorkerState, SectorWorkerJob) ([]*SectorState, error)

	RestoreSector func(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error)

	ReportFinalized func(context.Context, abi.SectorID) (Meta, error)

	ReportAborted func(context.Context, abi.SectorID, string) (Meta, error)

	CheckProvable func(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error)

	SimulateWdPoSt func(context.Context, address.Address, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error

	SnapUpPreFetch func(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*SnapUpFetchResult, error)

	SnapUpCandidates func(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)

	SnapUpCancelCommitment func(ctx context.Context, sid abi.SectorID) error

	ProvingSectorInfo func(ctx context.Context, sid abi.SectorID) (ProvingSectorInfo, error)

	WorkerGetPingInfo func(ctx context.Context, name string) (*WorkerPingInfo, error)

	WorkerPingInfoList func(ctx context.Context) ([]WorkerPingInfo, error)

	SectorIndexerFind func(ctx context.Context, indexType SectorIndexType, sid abi.SectorID) (SectorIndexLocation, error)

	TerminateSector func(context.Context, abi.SectorID) (SubmitTerminateResp, error)

	PollTerminateSectorState func(context.Context, abi.SectorID) (TerminateInfo, error)

	RemoveSector func(context.Context, abi.SectorID) error

	StoreReleaseReserved func(ctx context.Context, sid abi.SectorID) (bool, error)

	StoreList func(ctx context.Context) ([]StoreDetailedInfo, error)
}
