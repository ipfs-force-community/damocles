package core

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	verifregtypes "github.com/filecoin-project/venus/venus-shared/actors/builtin/verifreg"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"
	mtypes "github.com/filecoin-project/venus/venus-shared/types/market"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
	"github.com/ipfs/go-cid"
)

type AllocateSectorSpec struct {
	AllowedMiners     []abi.ActorID
	AllowedProofTypes []abi.RegisteredSealProof
}

type SnapUpCandidate struct {
	DeadlineIndex uint64
	Sector        AllocatedSector
	Public        SectorPublicInfo
	Private       SectorPrivateInfo
}

type AllocatedSector struct {
	ID        abi.SectorID
	ProofType abi.RegisteredSealProof
}

type SectorPiece interface {
	DisplayDealID() string
	PieceInfo() PieceInfo
	HasDealInfo() bool
	IsBuiltinMarket() bool
	DealID() abi.DealID
	AllocationID() verifregtypes.AllocationId
	Client() address.Address
	StartEpoch() abi.ChainEpoch
	EndEpoch() abi.ChainEpoch
}

type PieceInfo struct {
	Size   abi.PaddedPieceSize
	Offset abi.PaddedPieceSize
	Cid    cid.Cid
}

type LegacyDealInfo struct {
	ID          abi.DealID
	PayloadSize uint64
	Piece       PieceInfo
	Proposal    *DealProposal

	// this is the flag for pieces from original implementations
	// if true, workers should use the piece data directly, instead of padding themselves
	IsCompatible bool
}

func (ldi LegacyDealInfo) DisplayDealID() string {
	return fmt.Sprintf("builtinmarket(%d)", ldi.ID)
}

func (ldi LegacyDealInfo) PieceInfo() PieceInfo {
	return ldi.Piece
}

func (ldi LegacyDealInfo) HasDealInfo() bool {
	return ldi.ID != 0
}

func (ldi LegacyDealInfo) IsBuiltinMarket() bool {
	return ldi.HasDealInfo()
}

func (ldi LegacyDealInfo) DealID() abi.DealID {
	return ldi.ID
}

func (ldi LegacyDealInfo) AllocationID() verifregtypes.AllocationId {
	return verifregtypes.NoAllocationID
}

func (ldi LegacyDealInfo) Client() address.Address {
	if ldi.HasDealInfo() {
		return ldi.Proposal.Client
	}
	return address.Undef
}

// EndEpoch returns the minimum epoch until which the sector containing this
// deal must be committed until.
func (ldi LegacyDealInfo) StartEpoch() abi.ChainEpoch {
	if ldi.HasDealInfo() {
		return ldi.Proposal.StartEpoch
	}
	return abi.ChainEpoch(0)
}

// EndEpoch returns the minimum epoch until which the sector containing this
// deal must be committed until.
func (ldi LegacyDealInfo) EndEpoch() abi.ChainEpoch {
	if ldi.HasDealInfo() {
		return ldi.Proposal.StartEpoch
	}
	return abi.ChainEpoch(0)
}

type DealInfoV2 struct {
	*mtypes.DealInfoV2
	// if true, it indicates that the deal is an legacy builtin market deal,
	// otherwise it is a DDO deal
	IsBuiltinMarket bool
	// this is the flag for pieces from original implementations
	// if true, workers should use the piece data directly, instead of padding themselves
	IsCompatible bool
}

func (di DealInfoV2) DisplayID() string {
	if di.IsBuiltinMarket {
		return fmt.Sprintf("builtinmarket(%d)", di.DealID)
	}
	return fmt.Sprintf("ddo(%d)", di.AllocationID)
}

type SectorPieceV2 struct {
	Piece    abi.PieceInfo
	DealInfo *DealInfoV2
}

func (sp SectorPieceV2) PieceInfo() PieceInfo {
	if sp.HasDealInfo() {
		return PieceInfo{
			Size:   sp.Piece.Size,
			Offset: sp.DealInfo.Offset,
			Cid:    sp.Piece.PieceCID,
		}
	}
	return PieceInfo{
		Size: sp.Piece.Size,
		Cid:  sp.Piece.PieceCID,
	}
}

func (sp SectorPieceV2) DisplayDealID() string {
	return sp.DealInfo.DisplayID()
}

func (sp SectorPieceV2) HasDealInfo() bool {
	return sp.DealInfo != nil
}

func (sp SectorPieceV2) IsBuiltinMarket() bool {
	if !sp.HasDealInfo() {
		return false
	}

	return sp.DealInfo.IsBuiltinMarket
}

func (sp SectorPieceV2) DealID() abi.DealID {
	if sp.IsBuiltinMarket() {
		return sp.DealInfo.DealID
	}
	return abi.DealID(0)
}

func (sp SectorPieceV2) AllocationID() verifregtypes.AllocationId {
	if !sp.HasDealInfo() {
		return verifregtypes.NoAllocationID
	}

	return sp.DealInfo.AllocationID
}

func (sp SectorPieceV2) Client() address.Address {
	if !sp.HasDealInfo() {
		return address.Undef
	}

	return sp.DealInfo.Client
}

// EndEpoch returns the minimum epoch until which the sector containing this
// deal must be committed until.
func (sp SectorPieceV2) StartEpoch() abi.ChainEpoch {
	if sp.HasDealInfo() {
		return sp.DealInfo.StartEpoch
	}
	return abi.ChainEpoch(0)
}

// EndEpoch returns the minimum epoch until which the sector containing this
// deal must be committed until.
func (sp SectorPieceV2) EndEpoch() abi.ChainEpoch {
	if sp.HasDealInfo() {
		return sp.DealInfo.EndEpoch
	}
	return abi.ChainEpoch(0)
}

type Deals []LegacyDealInfo
type SectorPieces []SectorPieceV2

type AcquireDealsSpec struct {
	MaxDeals     *uint
	MinUsedSpace *uint64
}

type AcquireDealsLifetime struct {
	Start abi.ChainEpoch
	End   abi.ChainEpoch
	// filter by sector lifetime, for snapdeal
	SectorExpiration *abi.ChainEpoch
}

type Ticket struct {
	Ticket abi.Randomness
	Epoch  abi.ChainEpoch
}

type SubmitResult uint64

const (
	SubmitUnknown SubmitResult = iota
	SubmitAccepted
	SubmitDuplicateSubmit
	// worker should enter perm err
	SubmitMismatchedSubmission
	// worker should enter perm err
	SubmitRejected
	// worker should retry persisting files
	SubmitFilesMissed
)

type OnChainState uint64

const (
	OnChainStateUnknown OnChainState = iota
	OnChainStatePending
	OnChainStatePacked
	OnChainStateLanded
	OnChainStateNotFound
	// worker would try re-submit the info
	OnChainStateFailed
	// worker should enter perm err
	OnChainStatePermFailed
	OnChainStateShouldAbort
)

type PreCommitOnChainInfo struct {
	CommR  [32]byte
	CommD  [32]byte
	Ticket Ticket
}

func (pi PreCommitOnChainInfo) IntoPreCommitInfo() (PreCommitInfo, error) {
	commR, err := commcid.ReplicaCommitmentV1ToCID(pi.CommR[:])
	if err != nil {
		return PreCommitInfo{}, err
	}

	commD, err := commcid.DataCommitmentV1ToCID(pi.CommD[:])
	if err != nil {
		return PreCommitInfo{}, err
	}

	return PreCommitInfo{
		CommR:  commR,
		CommD:  commD,
		Ticket: pi.Ticket,
	}, nil
}

type ProofOnChainInfo struct {
	Proof []byte
}

type SubmitPreCommitResp struct {
	Res  SubmitResult
	Desc *string
}

type PollPreCommitStateResp struct {
	State OnChainState
	Desc  *string
}

type WaitSeedResp struct {
	ShouldWait bool
	Delay      int
	Seed       *Seed
}

type Seed struct {
	Seed  abi.Randomness
	Epoch abi.ChainEpoch
}

type SubmitProofResp struct {
	Res  SubmitResult
	Desc *string
}

type PollProofStateResp struct {
	State OnChainState
	Desc  *string
}

type SubmitTerminateResp struct {
	Res  SubmitResult
	Desc *string
}

type MinerInfo struct {
	ID   abi.ActorID
	Addr address.Address
	// Addr                address.Address
	// Owner               address.Address
	// Worker              address.Address
	// NewWorker           address.Address
	// ControlAddresses    []address.Address
	// WorkerChangeEpoch   abi.ChainEpoch
	SectorSize          abi.SectorSize
	WindowPoStProofType abi.RegisteredPoStProof
	SealProofType       abi.RegisteredSealProof
}

type TipSetToken []byte

type PreCommitInfo struct {
	CommR  cid.Cid
	CommD  cid.Cid
	Ticket Ticket
}

func (pc PreCommitInfo) IntoPreCommitOnChainInfo() (PreCommitOnChainInfo, error) {
	commR, err := commcid.CIDToReplicaCommitmentV1(pc.CommR)
	if err != nil {
		return PreCommitOnChainInfo{}, fmt.Errorf("convert to replica commitment: %w", err)
	}

	commD, err := commcid.CIDToDataCommitmentV1(pc.CommD)
	if err != nil {
		return PreCommitOnChainInfo{}, fmt.Errorf("convert to data commitment: %w", err)
	}

	info := PreCommitOnChainInfo{
		Ticket: pc.Ticket,
	}
	copy(info.CommR[:], commR)
	copy(info.CommD[:], commD)

	return info, nil
}

type ProofInfo = ProofOnChainInfo

type AggregateInput struct {
	Spt   abi.RegisteredSealProof
	Info  AggregateSealVerifyInfo
	Proof []byte
}

type PreCommitEntry struct {
	Deposit     abi.TokenAmount
	Pcsp        *miner.SectorPreCommitInfo
	SectorState *SectorState
}

type MessageInfo struct {
	PreCommitCid *cid.Cid
	CommitCid    *cid.Cid
	NeedSend     bool
}

type TerminateInfo struct {
	TerminateCid *cid.Cid
	TerminatedAt abi.ChainEpoch
	AddedHeight  abi.ChainEpoch
}

type ReportStateReq struct {
	Worker      WorkerIdentifier
	StateChange SectorStateChange
	Failure     *SectorFailure
}

type WorkerIdentifier struct {
	Instance string
	Location string
}

type SectorStateChange struct {
	Prev  string
	Next  string
	Event string
}

type SectorFailure struct {
	Level string
	Desc  string
}

type WindowPoStRandomness struct {
	Epoch abi.ChainEpoch
	Rand  abi.Randomness
}

type ActorIdent struct {
	ID   abi.ActorID
	Addr address.Address
}

type AllocateSnapUpSpec struct {
	Sector AllocateSectorSpec
	Deals  AcquireDealsSpec
}

type SectorPublicInfo struct {
	CommR      [32]byte
	SealedCID  cid.Cid
	Activation abi.ChainEpoch
	Expiration abi.ChainEpoch
}

type SectorPrivateInfo struct {
	AccessInstance string
}

type AllocatedSnapUpSector struct {
	Sector  AllocatedSector
	Pieces  SectorPieces
	Public  SectorPublicInfo
	Private SectorPrivateInfo
}

type SnapUpOnChainInfo struct {
	CommR          [32]byte
	CommD          [32]byte
	AccessInstance string
	Pieces         []cid.Cid
	Proof          []byte
}

type SubmitSnapUpProofResp struct {
	Res  SubmitResult
	Desc *string
}

type SnapUpFetchResult struct {
	Total uint64
	Diff  uint64
}

type ProvingSectorInfo struct {
	OnChain SectorOnChainInfo
	Private PrivateSectorInfo
}

type SectorIndexType string

const (
	SectorIndexTypeNormal  SectorIndexType = "normal"
	SectorIndexTypeUpgrade SectorIndexType = "upgrade"
)

type SectorIndexLocation struct {
	Found    bool
	Instance SectorAccessStores
}

type SectorAccessStores struct {
	SealedFile string // name for storage instance
	CacheDir   string
}

type StoreBasicInfo struct {
	Name     string
	Path     string
	Strict   bool
	ReadOnly bool
	Weight   uint
	Meta     map[string]string
}

type StoreDetailedInfo struct {
	StoreBasicInfo
	Type        string
	Total       uint64
	Free        uint64
	Used        uint64
	UsedPercent float64
	Reserved    uint64
	ReservedBy  []ReservedItem
}

type ReservedItem = objstore.StoreReserved

type RebuildOptions struct {
	PiecesAvailable bool
}

type SectorRebuildInfo struct {
	Sector        AllocatedSector
	Ticket        Ticket
	Pieces        Deals
	PiecesV2      SectorPieces
	IsSnapUp      bool
	UpgradePublic *SectorUpgradePublic
}

type UnsealTaskIdentifier struct {
	PieceCid     cid.Cid
	Actor        abi.ActorID
	SectorNumber abi.SectorNumber
}

type SectorUnsealInfo struct {
	Sector   AllocatedSector
	PieceCid cid.Cid
	Offset   vtypes.UnpaddedByteIndex
	Size     abi.UnpaddedPieceSize

	// todo: set payload size to unseal task and droplet request
	// PayloadSize uint64

	PrivateInfo SectorPrivateInfo
	Ticket      Ticket
	CommD       [32]byte

	// there may be more than one unseal event result into on unseal task
	Dest      []string
	State     gtypes.UnsealState
	ErrorInfo string
}

type SectorStateResp struct {
	ID          abi.SectorID `json:"Id"`
	Finalized   SectorFinalized
	AbortReason *string
}

func UnsealInfoKey(actor abi.ActorID, sectorNumber abi.SectorNumber, pieceCid cid.Cid) string {
	return fmt.Sprintf("%d-%d-%s", actor, sectorNumber, pieceCid.String())
}

func (s UnsealTaskIdentifier) String() string {
	return fmt.Sprintf("%d-%d-%s", s.Actor, s.SectorNumber, s.PieceCid.String())
}
