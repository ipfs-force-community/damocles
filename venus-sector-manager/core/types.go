package core

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
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

type PieceInfo struct {
	Size abi.PaddedPieceSize
	Cid  cid.Cid
}

type DealInfo struct {
	ID          abi.DealID
	PayloadSize uint64
	Piece       PieceInfo
	Proposal    *DealProposal

	// this is the flag for pieces from original implmentations
	// if true, workers should use the piece data directly, instead of padding themselves
	IsCompatible bool
}

type Deals []DealInfo

type AcquireDealsSpec struct {
	MaxDeals     *uint
	MinUsedSpace *uint64
}

type AcquireDealsLifetime struct {
	Start abi.ChainEpoch
	End   abi.ChainEpoch
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
	// worker whould try re-submit the info
	OnChainStateFailed
	// worker should enter perm err
	OnChainStatePermFailed
	OnChainStateShouldAbort
)

type PreCommitOnChainInfo struct {
	CommR  [32]byte
	CommD  [32]byte
	Ticket Ticket
	Deals  []abi.DealID
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
		Deals:  pi.Deals,
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
	Deals  []abi.DealID
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
		Deals:  pc.Deals,
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
	Deposit abi.TokenAmount
	Pci     *miner.SectorPreCommitInfo
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
	Pieces  Deals
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
	SealedFile string
	CacheDir   string
}

type StoreBasicInfo struct {
	Name string
	Path string
	Meta map[string]string
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
	IsSnapUp      bool
	UpgradePublic *SectorUpgradePublic
}
