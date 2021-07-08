package commitmgr

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/market"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
	"github.com/ipfs/go-cid"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var ErrSectorAllocated = errors.New("sectorNumber is allocated, but PreCommit info wasn't found on chain")

type SealingAPI interface {
	StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tok api.TipSetToken) (cid.Cid, error)

	// Can return ErrSectorAllocated in case precommit info wasn't found, but the sector number is marked as allocated
	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok api.TipSetToken) (*miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok api.TipSetToken) (*miner.SectorOnChainInfo, error)
	StateMinerSectorSize(context.Context, address.Address, api.TipSetToken) (abi.SectorSize, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, miner.SectorPreCommitInfo, api.TipSetToken) (big.Int, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, api.TipSetToken) (big.Int, error)
	StateMarketStorageDealProposal(context.Context, abi.DealID, api.TipSetToken) (market.DealProposal, error)
	StateMinerInfo(context.Context, address.Address, api.TipSetToken) (miner.MinerInfo, error)
	StateMinerSectorAllocated(context.Context, address.Address, abi.SectorNumber, api.TipSetToken) (bool, error)
	StateNetworkVersion(ctx context.Context, tok api.TipSetToken) (network.Version, error)
	ChainHead(ctx context.Context) (api.TipSetToken, abi.ChainEpoch, error)
	ChainBaseFee(ctx context.Context, tok api.TipSetToken) (abi.TokenAmount, error)

	// validate random, may need to consider the place here
	ChainGetRandomnessFromBeacon(ctx context.Context, tok api.TipSetToken, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
}

type Verifier interface {
	VerifySeal(proof5.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error)
}

type Prover interface {
	AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error)
}
