package commitmgr

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stminer "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin/market"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/verifreg"
	"github.com/filecoin-project/venus/venus-shared/blockstore"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

type SealingAPIImpl struct {
	api chainapi.API
	core.RandomnessAPI
}

func NewSealingAPIImpl(api chainapi.API, rand core.RandomnessAPI) SealingAPIImpl {
	return SealingAPIImpl{
		api:           api,
		RandomnessAPI: rand,
	}
}

func (s SealingAPIImpl) StateComputeDataCommitment(
	ctx context.Context,
	maddr address.Address,
	sectorType abi.RegisteredSealProof,
	deals []abi.DealID,
	tok core.TipSetToken,
) (cid.Cid, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	return s.api.StateComputeDataCID(ctx, maddr, sectorType, deals, tsk)
}

func (s SealingAPIImpl) StateSectorPreCommitInfo(
	ctx context.Context,
	maddr address.Address,
	sectorNumber abi.SectorNumber,
	tok core.TipSetToken,
) (*stminer.SectorPreCommitOnChainInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	act, err := s.api.StateGetActor(ctx, maddr, tsk)
	if err != nil {
		return nil, fmt.Errorf("handleSealFailed(%d): temp error: %w", sectorNumber, err)
	}
	stor := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(s.api)))

	state, err := miner.Load(stor, act)
	if err != nil {
		return nil, fmt.Errorf("handleSealFailed(%d): temp error: loading miner state: %w", sectorNumber, err)
	}

	pci, err := state.GetPrecommittedSector(sectorNumber)
	if err != nil {
		return nil, err
	}
	if pci == nil {
		set, err := state.IsAllocated(sectorNumber)
		if err != nil {
			return nil, fmt.Errorf("checking if sector is allocated: %w", err)
		}
		if set {
			return nil, ErrSectorAllocated
		}

		return nil, nil
	}

	return pci, nil
}

func (s SealingAPIImpl) StateSectorGetInfo(
	ctx context.Context,
	maddr address.Address,
	sectorNumber abi.SectorNumber,
	tok core.TipSetToken,
) (*miner.SectorOnChainInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}
	return s.api.StateSectorGetInfo(ctx, maddr, sectorNumber, tsk)
}

func (s SealingAPIImpl) StateMinerSectorSize(
	ctx context.Context,
	maddr address.Address,
	tok core.TipSetToken,
) (abi.SectorSize, error) {
	mi, err := s.StateMinerInfo(ctx, maddr, tok)
	if err != nil {
		return 0, err
	}
	return mi.SectorSize, nil
}

func (s SealingAPIImpl) StateMinerPreCommitDepositForPower(
	ctx context.Context,
	address address.Address,
	info stminer.SectorPreCommitInfo,
	token core.TipSetToken,
) (big.Int, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return big.Zero(), fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	return s.api.StateMinerPreCommitDepositForPower(ctx, address, info, tsk)
}

func (s SealingAPIImpl) StateMinerInitialPledgeCollateral(
	ctx context.Context,
	address address.Address,
	info stminer.SectorPreCommitInfo,
	token core.TipSetToken,
) (big.Int, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return big.Zero(), fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	return s.api.StateMinerInitialPledgeCollateral(ctx, address, info, tsk)
}

func (s SealingAPIImpl) StateMarketStorageDealProposal(
	ctx context.Context,
	id abi.DealID,
	token core.TipSetToken,
) (market.DealProposal, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return market.DealProposal{}, err
	}

	deal, err := s.api.StateMarketStorageDeal(ctx, id, tsk)
	if err != nil {
		return market.DealProposal{}, err
	}

	return deal.Proposal, nil
}

func (s SealingAPIImpl) StateGetAllocationIdForPendingDeal(
	ctx context.Context,
	dealID abi.DealID,
	tst core.TipSetToken,
) (verifreg.AllocationId, error) {
	tsk, err := types.TipSetKeyFromBytes(tst)
	if err != nil {
		return verifreg.AllocationId(0), err
	}
	return s.api.StateGetAllocationIdForPendingDeal(ctx, dealID, tsk)
}

func (s SealingAPIImpl) StateGetAllocationForPendingDeal(
	ctx context.Context,
	dealID abi.DealID,
	tst core.TipSetToken,
) (*types.Allocation, error) {
	tsk, err := types.TipSetKeyFromBytes(tst)
	if err != nil {
		return nil, err
	}
	return s.api.StateGetAllocationForPendingDeal(ctx, dealID, tsk)
}

func (s SealingAPIImpl) StateGetAllocation(
	ctx context.Context,
	clientAddr address.Address,
	allocationID types.AllocationId,
	tst core.TipSetToken,
) (*types.Allocation, error) {
	tsk, err := types.TipSetKeyFromBytes(tst)
	if err != nil {
		return nil, err
	}
	return s.api.StateGetAllocation(ctx, clientAddr, allocationID, tsk)
}

func (s SealingAPIImpl) StateMinerInfo(
	ctx context.Context,
	address address.Address,
	token core.TipSetToken,
) (types.MinerInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return types.MinerInfo{}, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	// TODO: update storage-fsm to just StateMinerInfo
	return s.api.StateMinerInfo(ctx, address, tsk)
}

func (s SealingAPIImpl) StateMinerSectorAllocated(
	ctx context.Context,
	address address.Address,
	number abi.SectorNumber,
	token core.TipSetToken,
) (bool, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	return s.api.StateMinerSectorAllocated(ctx, address, number, tsk)
}

func (s SealingAPIImpl) StateNetworkVersion(ctx context.Context, tok core.TipSetToken) (network.Version, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return network.VersionMax, err
	}

	return s.api.StateNetworkVersion(ctx, tsk)
}

func (s SealingAPIImpl) ChainHead(ctx context.Context) (core.TipSetToken, abi.ChainEpoch, error) {
	head, err := s.api.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

func (s SealingAPIImpl) ChainBaseFee(ctx context.Context, tok core.TipSetToken) (abi.TokenAmount, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return big.Zero(), err
	}

	ts, err := s.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return big.Zero(), err
	}

	return ts.Blocks()[0].ParentBaseFee, nil
}

func (s SealingAPIImpl) StateSectorPartition(
	ctx context.Context,
	maddr address.Address,
	sectorNumber abi.SectorNumber,
	tok core.TipSetToken,
) (*miner.SectorLocation, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	l, err := s.api.StateSectorPartition(ctx, maddr, sectorNumber, tsk)
	if err != nil {
		return nil, err
	}
	if l != nil {
		return &miner.SectorLocation{
			Deadline:  l.Deadline,
			Partition: l.Partition,
		}, nil
	}

	return nil, nil // not found
}

func (s SealingAPIImpl) StateMinerPartitions(
	ctx context.Context,
	maddr address.Address,
	dlIdx uint64,
	tok core.TipSetToken,
) ([]chainapi.Partition, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %w", err)
	}

	return s.api.StateMinerPartitions(ctx, maddr, dlIdx, tsk)
}

func (s SealingAPIImpl) StateMinerProvingDeadline(
	ctx context.Context,
	maddr address.Address,
	tok core.TipSetToken,
) (*dline.Info, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, err
	}

	return s.api.StateMinerProvingDeadline(ctx, maddr, tsk)
}

var _ SealingAPI = (*SealingAPIImpl)(nil)
