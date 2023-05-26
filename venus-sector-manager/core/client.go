package core

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"
)

var UnavailableSealerCliClient = SealerCliClient{
	ListSectors: func(context.Context, SectorWorkerState, SectorWorkerJob) ([]*SectorState, error) {
		panic("sealer client unavailable")
	},

	FindSector: func(ctx context.Context, state SectorWorkerState, sid abi.SectorID) (*SectorState, error) {
		panic("sealer client unavailable")
	},

	FindSectorsWithDeal: func(ctx context.Context, state SectorWorkerState, dealID abi.DealID) ([]*SectorState, error) {
		panic("sealer client unavailable")
	},

	FindSectorWithPiece: func(ctx context.Context, state SectorWorkerState, pieceCid cid.Cid) (*SectorState, error) {
		panic("sealer client unavailable")
	},

	ImportSector: func(ctx context.Context, ws SectorWorkerState, state *SectorState, override bool) (bool, error) {
		panic("sealer client unavailable")
	},

	RestoreSector: func(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error) {
		panic("sealer client unavailable")
	},

	ReportFinalized: func(context.Context, abi.SectorID) (Meta, error) { panic("sealer client unavailable") },

	ReportAborted: func(context.Context, abi.SectorID, string) (Meta, error) { panic("sealer client unavailable") },

	CheckProvable: func(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict, stateCheck bool) (map[abi.SectorNumber]string, error) {
		panic("sealer client unavailable")
	},

	SimulateWdPoSt: func(context.Context, address.Address, abi.RegisteredPoStProof, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error {
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

	WorkerPingInfoRemove: func(ctx context.Context, name string) error {
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

	FinalizeSector: func(context.Context, abi.SectorID) error {
		panic("sealer client unavailable")
	},

	StoreReleaseReserved: func(ctx context.Context, sid abi.SectorID) (bool, error) {
		panic("sealer client unavailable")
	},

	StoreList: func(ctx context.Context) ([]StoreDetailedInfo, error) {
		panic("sealer client unavailable")
	},

	SectorSetForRebuild: func(ctx context.Context, sid abi.SectorID, opt RebuildOptions) (bool, error) {
		panic("sealer client unavailable")
	},

	// not listed in SealerCliAPI, but required in cli commands
	SubmitPreCommit: func(context.Context, AllocatedSector, PreCommitOnChainInfo, bool) (SubmitPreCommitResp, error) {
		panic("sealer client unavailable")
	},

	SubmitProof: func(context.Context, abi.SectorID, ProofOnChainInfo, bool) (SubmitProofResp, error) {
		panic("sealer client unavailable")
	},

	UnsealPiece: func(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, offset types.UnpaddedByteIndex, size abi.UnpaddedPieceSize, dest string) (<-chan []byte, error) {
		panic("sealer client unavailable")
	},
}

type SealerCliClient struct {
	ListSectors func(context.Context, SectorWorkerState, SectorWorkerJob) ([]*SectorState, error)

	FindSector func(ctx context.Context, state SectorWorkerState, sid abi.SectorID) (*SectorState, error)

	FindSectorsWithDeal func(ctx context.Context, state SectorWorkerState, dealID abi.DealID) ([]*SectorState, error)

	FindSectorWithPiece func(ctx context.Context, state SectorWorkerState, pieceCid cid.Cid) (*SectorState, error)

	ImportSector func(ctx context.Context, ws SectorWorkerState, state *SectorState, override bool) (bool, error)

	RestoreSector func(ctx context.Context, sid abi.SectorID, forced bool) (Meta, error)

	ReportFinalized func(context.Context, abi.SectorID) (Meta, error)

	ReportAborted func(context.Context, abi.SectorID, string) (Meta, error)

	CheckProvable func(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict, stateCheck bool) (map[abi.SectorNumber]string, error)

	SimulateWdPoSt func(context.Context, address.Address, abi.RegisteredPoStProof, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error

	SnapUpPreFetch func(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*SnapUpFetchResult, error)

	SnapUpCandidates func(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error)

	SnapUpCancelCommitment func(ctx context.Context, sid abi.SectorID) error

	ProvingSectorInfo func(ctx context.Context, sid abi.SectorID) (ProvingSectorInfo, error)

	WorkerGetPingInfo func(ctx context.Context, name string) (*WorkerPingInfo, error)

	WorkerPingInfoList func(ctx context.Context) ([]WorkerPingInfo, error)

	WorkerPingInfoRemove func(ctx context.Context, name string) error

	SectorIndexerFind func(ctx context.Context, indexType SectorIndexType, sid abi.SectorID) (SectorIndexLocation, error)

	TerminateSector func(context.Context, abi.SectorID) (SubmitTerminateResp, error)

	PollTerminateSectorState func(context.Context, abi.SectorID) (TerminateInfo, error)

	RemoveSector func(context.Context, abi.SectorID) error

	FinalizeSector func(context.Context, abi.SectorID) error

	StoreReleaseReserved func(ctx context.Context, sid abi.SectorID) (bool, error)

	StoreList func(ctx context.Context) ([]StoreDetailedInfo, error)

	SectorSetForRebuild func(ctx context.Context, sid abi.SectorID, opt RebuildOptions) (bool, error)

	// not listed in SealerCliAPI, but required in cli commands
	SubmitPreCommit func(context.Context, AllocatedSector, PreCommitOnChainInfo, bool) (SubmitPreCommitResp, error)

	SubmitProof func(context.Context, abi.SectorID, ProofOnChainInfo, bool) (SubmitProofResp, error)

	UnsealPiece func(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, offset types.UnpaddedByteIndex, size abi.UnpaddedPieceSize, dest string) (<-chan []byte, error)
}

var UnavailableMinerAPIClient = MinerAPIClient{
	GetInfo: func(context.Context, abi.ActorID) (*MinerInfo, error) {
		panic("damocles miner client unavailable")
	},
	GetMinerConfig: func(context.Context, abi.ActorID) (*modules.MinerConfig, error) {
		panic("damocles miner client unavailable")
	},
}

type MinerAPIClient struct {
	GetInfo        func(context.Context, abi.ActorID) (*MinerInfo, error)
	GetMinerConfig func(context.Context, abi.ActorID) (*modules.MinerConfig, error)
}
