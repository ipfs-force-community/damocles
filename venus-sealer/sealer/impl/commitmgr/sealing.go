package commitmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"
	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/filecoin-project/venus/pkg/specactors"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/market"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	miner1 "github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	chainAPI "github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type SealingAPIImpl struct {
	api chainAPI.API
}

func NewSealingAPIImpl(api chainAPI.API) SealingAPIImpl {
	return SealingAPIImpl{
		api: api,
	}
}

func (s SealingAPIImpl) StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tok api.TipSetToken) (cid.Cid, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	nv, err := s.api.StateNetworkVersion(ctx, tsk)
	if err != nil {
		return cid.Cid{}, err
	}

	var ccparams []byte
	if nv < network.Version13 {
		ccparams, err = specactors.SerializeParams(&market2.ComputeDataCommitmentParams{
			DealIDs:    deals,
			SectorType: sectorType,
		})
	} else {
		ccparams, err = specactors.SerializeParams(&market5.ComputeDataCommitmentParams{
			Inputs: []*market5.SectorDataSpec{
				{
					DealIDs:    deals,
					SectorType: sectorType,
				},
			},
		})
	}

	if err != nil {
		return cid.Undef, fmt.Errorf("computing params for ComputeDataCommitment: %s", err)
	}

	ccmt := &types.Message{
		To:     builtin5.StorageMarketActorAddr,
		From:   maddr,
		Value:  types.NewInt(0),
		Method: builtin5.MethodsMarket.ComputeDataCommitment,
		Params: ccparams,
	}
	r, err := s.api.StateCall(ctx, ccmt, tsk)
	if err != nil {
		return cid.Undef, fmt.Errorf("calling ComputeDataCommitment: %s", err)
	}
	if r.MsgRct.ExitCode != 0 {
		return cid.Undef, fmt.Errorf("receipt for ComputeDataCommitment had exit code %d", r.MsgRct.ExitCode)
	}

	if nv < network.Version13 {
		var c cbg.CborCid
		if err := c.UnmarshalCBOR(bytes.NewReader(r.MsgRct.ReturnValue)); err != nil {
			return cid.Undef, fmt.Errorf("failed to unmarshal CBOR to CborCid: %s", err)
		}

		return cid.Cid(c), nil
	}

	var cr market5.ComputeDataCommitmentReturn
	if err := cr.UnmarshalCBOR(bytes.NewReader(r.MsgRct.ReturnValue)); err != nil {
		return cid.Undef, fmt.Errorf("failed to unmarshal CBOR to CborCid: %s", err)
	}

	if len(cr.CommDs) != 1 {
		return cid.Undef, fmt.Errorf("CommD output must have 1 entry")
	}

	return cid.Cid(cr.CommDs[0]), nil
}

func (s SealingAPIImpl) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok api.TipSetToken) (*miner.SectorPreCommitOnChainInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	act, err := s.api.StateGetActor(ctx, maddr, tsk)
	if err != nil {
		return nil, fmt.Errorf("handleSealFailed(%d): temp error: %+v", sectorNumber, err)
	}
	stor := chain.ActorStore(ctx, chainAPI.NewAPIBlockstore(s.api))

	state, err := miner1.Load(stor, act)
	if err != nil {
		return nil, fmt.Errorf("handleSealFailed(%d): temp error: loading miner state: %+v", sectorNumber, err)
	}

	pci, err := state.GetPrecommittedSector(sectorNumber)
	if err != nil {
		return nil, err
	}
	if pci == nil {
		set, err := state.IsAllocated(sectorNumber)
		if err != nil {
			return nil, fmt.Errorf("checking if sector is allocated: %s", err)
		}
		if set {
			return nil, ErrSectorAllocated
		}

		return nil, nil
	}

	return pci, nil
}

func (s SealingAPIImpl) StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok api.TipSetToken) (*miner.SectorOnChainInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}
	return s.api.StateSectorGetInfo(ctx, maddr, sectorNumber, tsk)
}

func (s SealingAPIImpl) StateMinerSectorSize(ctx context.Context, maddr address.Address, tok api.TipSetToken) (abi.SectorSize, error) {
	mi, err := s.StateMinerInfo(ctx, maddr, tok)
	if err != nil {
		return 0, err
	}
	return mi.SectorSize, nil
}

func (s SealingAPIImpl) StateMinerPreCommitDepositForPower(ctx context.Context, address address.Address, info miner.SectorPreCommitInfo, token api.TipSetToken) (big.Int, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return big.Zero(), fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	return s.api.StateMinerPreCommitDepositForPower(ctx, address, info, tsk)
}

func (s SealingAPIImpl) StateMinerInitialPledgeCollateral(ctx context.Context, address address.Address, info miner.SectorPreCommitInfo, token api.TipSetToken) (big.Int, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return big.Zero(), fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	return s.api.StateMinerInitialPledgeCollateral(ctx, address, info, tsk)
}

func (s SealingAPIImpl) StateMarketStorageDealProposal(ctx context.Context, id abi.DealID, token api.TipSetToken) (market.DealProposal, error) {
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

func (s SealingAPIImpl) StateMinerInfo(ctx context.Context, address address.Address, token api.TipSetToken) (miner.MinerInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return miner.MinerInfo{}, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	// TODO: update storage-fsm to just StateMinerInfo
	return s.api.StateMinerInfo(ctx, address, tsk)
}

func (s SealingAPIImpl) StateMinerSectorAllocated(ctx context.Context, address address.Address, number abi.SectorNumber, token api.TipSetToken) (bool, error) {
	tsk, err := types.TipSetKeyFromBytes(token)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal TipSetToken to TipSetKey: %s", err)
	}

	return s.api.StateMinerSectorAllocated(ctx, address, number, tsk)
}

func (s SealingAPIImpl) StateNetworkVersion(ctx context.Context, tok api.TipSetToken) (network.Version, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return network.VersionMax, err
	}

	return s.api.StateNetworkVersion(ctx, tsk)
}

func (s SealingAPIImpl) ChainHead(ctx context.Context) (api.TipSetToken, abi.ChainEpoch, error) {
	head, err := s.api.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

func (s SealingAPIImpl) ChainBaseFee(ctx context.Context, tok api.TipSetToken) (abi.TokenAmount, error) {
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

func (s SealingAPIImpl) ChainGetRandomnessFromBeacon(ctx context.Context, tok api.TipSetToken, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return nil, err
	}

	return s.api.ChainGetRandomnessFromBeacon(ctx, tsk, personalization, randEpoch, entropy)
}

var _ SealingAPI = (*SealingAPIImpl)(nil)
